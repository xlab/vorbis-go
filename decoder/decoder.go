// Package decoder implements an OggVorbis decoder. Based on libogg/libvorbis bindings.
package decoder

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/xlab/vorbis-go/vorbis"
)

const (
	// OUT_BUFFER_SIZE defines the number of frames buffered in the PCM output channel.
	OUT_BUFFER_SIZE = 8
	// DATA_CHUNK_SIZE represents the amount of data read from physical bitstream on each iteration.
	DATA_CHUNK_SIZE = 4096 // could be also 8192
)

// Decoder implements an OggVorbis decoder.
type Decoder struct {
	sync.Mutex

	// syncState tracks the synchronization of the current page. It is used during
	// decoding to track the status of data as it is read in, synchronized, verified,
	// and parsed into pages belonging to the various logical bistreams
	// in the current physical bitstream link.
	syncState vorbis.OggSyncState

	// streamState tracks the current decode state of the current logical bitstream.
	streamState vorbis.OggStreamState

	// page encapsulates the data for an Ogg page. Ogg pages are the fundamental unit
	// of framing and interleave in an Ogg bitstream.
	page vorbis.OggPage

	// packet encapsulates the data for a single raw packet of data and is used to transfer
	// data between the Ogg framing layer and the handling codec.
	packet vorbis.OggPacket

	// info contains basic information about the audio in a vorbis bitstream.
	info vorbis.Info

	// comment stores all the bitstream user comments as Ogg Vorbis comment.
	comment vorbis.Comment

	// dspState is the state for one instance of the Vorbis decoder.
	// This structure is intended to be private.
	dspState vorbis.DspState

	// block holds the data for a single block of audio. One Vorbis block translates to one codec packet.
	// The decoding process consists of decoding the packets into blocks and reassembling the audio from the blocks.
	// This structure is intended to be private.
	block vorbis.Block

	// samplesPerChannel defines the exact number of samples per channel in a frame.
	// All partial frames should be merged, if possible, to meet this constraint.
	samplesPerChannel int

	input    io.Reader
	pcmOut   chan [][]float32
	stopChan chan struct{}
	closed   bool
	onError  func(err error)
}

// Info represents basic information about the audio in a Vorbis bitstream.
type Info struct {
	Channels   int32
	SampleRate float64
	Comments   []string
	Vendor     string
}

// New creates and initialises a new OggVorbis decoder for the provided bytestream.
func New(r io.Reader, samplesPerChannel int) (*Decoder, error) {
	d := &Decoder{
		samplesPerChannel: samplesPerChannel,

		input:    r,
		pcmOut:   make(chan [][]float32, OUT_BUFFER_SIZE),
		stopChan: make(chan struct{}),
	}
	vorbis.OggSyncInit(&d.syncState)
	if err := d.readStreamHeaders(r); err != nil {
		d.decoderStateCleanup()
		return nil, err
	}
	return d, nil
}

// Info returns some basic info about the Vorbis stream the decoder was fed with.
func (d *Decoder) Info() Info {
	info := Info{
		Channels:   d.info.Channels,
		SampleRate: float64(d.info.Rate),
		Vendor:     toString(d.comment.Vendor, 256),
	}
	lengths := d.comment.CommentLengths[:d.comment.Comments]
	userComments := d.comment.UserComments[:d.comment.Comments]
	for i, text := range userComments {
		info.Comments = append(info.Comments, string(text[:lengths[i]]))
	}
	return info
}

// SetErrorHandler sets the callback function that will be called upon a decode error occurs.
func (d *Decoder) SetErrorHandler(fn func(err error)) {
	d.onError = fn
}

func (d *Decoder) reportError(err error) {
	if d.onError != nil {
		d.onError(err)
	}
}

// SamplesOut is a read-only channel of sample frames, each frame contains exactly
// samplesPerChannel samples as has been specified, unless it is an EOS situation
// where the last frame has the last chunk of samples.
// The PCM sample format is float32.
//
// Example: 2 channels, 4096 samples per channel would result in [4096][2]float32.
func (d *Decoder) SamplesOut() <-chan [][]float32 {
	return d.pcmOut
}

// Close stops and finalizes the decoding process, releases the allocated resources.
// Puts the decoder into an unrecoverable state.
func (d *Decoder) Close() {
	if !d.stopRequested() {
		close(d.stopChan)
	}
	d.Lock()
	defer d.Unlock()
	if d.closed {
		return
	}
	d.closed = true
	close(d.pcmOut)
	d.decoderStateCleanup()
}

func (d *Decoder) decoderStateCleanup() {
	vorbis.OggStreamClear(&d.streamState)
	d.streamState.Free()

	vorbis.CommentClear(&d.comment)
	d.comment.Free()

	vorbis.InfoClear(&d.info)
	d.info.Free()

	vorbis.OggSyncDestroy(&d.syncState)
	d.syncState.Free()

	// clear up all remaining refs
	d.packet.Free()
	d.page.Free()
}

func (d *Decoder) stopRequested() bool {
	select {
	case <-d.stopChan:
		return true
	default:
		return false
	}
}

// readPage reads a page of exact size into libvorbis' Ogg layer.
func (d *Decoder) readChunk(r io.Reader) (n int, err error) {
	buf := vorbis.OggSyncBuffer(&d.syncState, DATA_CHUNK_SIZE)
	n, err = io.ReadFull(r, buf[:DATA_CHUNK_SIZE])
	vorbis.OggSyncWrote(&d.syncState, int(n))
	if err == io.ErrUnexpectedEOF {
		return n, io.EOF
	}
	return n, err
}

func (d *Decoder) readStreamHeaders(r io.Reader) error {
	d.readChunk(r)

	// Read the first page
	if ret := vorbis.OggSyncPageout(&d.syncState, &d.page); ret != 1 {
		return errors.New("vorbis: not a valid Ogg bitstream")
	}

	// Init the logical bitstream with serial number stored in the page
	vorbis.OggStreamInit(&d.streamState, vorbis.OggPageSerialno(&d.page))

	vorbis.InfoInit(&d.info)
	vorbis.CommentInit(&d.comment)

	// Add a complete page to the bitstream
	if ret := vorbis.OggStreamPagein(&d.streamState, &d.page); ret < 0 {
		return errors.New("vorbis: the supplied page does not belong this Vorbis stream")
	}
	// Get the first packet
	if ret := vorbis.OggStreamPacketout(&d.streamState, &d.packet); ret != 1 {
		return errors.New("vorbis: unable to fetch initial Vorbis packet from the first page")
	}
	// Finally decode the header packet
	if ret := vorbis.SynthesisHeaderin(&d.info, &d.comment, &d.packet); ret < 0 {
		return fmt.Errorf("vorbis: unable to decode the initial Vorbis header: %d", ret)
	}

	var headersRead int
forPage:
	for headersRead < 2 {
		if res := vorbis.OggSyncPageout(&d.syncState, &d.page); res < 0 {
			// bytes have been skipped, try to sync again
			continue forPage
		} else if res == 0 {
			// go get more data
			if _, err := d.readChunk(r); err != nil {
				return errors.New("vorbis: got EOF while reading Vorbis headers")
			}
			continue forPage
		}
		// page is synced at this point
		vorbis.OggStreamPagein(&d.streamState, &d.page)
		for headersRead < 2 {
			if ret := vorbis.OggStreamPacketout(&d.streamState, &d.packet); ret < 0 {
				return errors.New("vorbis: data is missing near the secondary Vorbis header")
			} else if ret == 0 {
				// no packets left on the page, go get a new one
				continue forPage
			}
			if ret := vorbis.SynthesisHeaderin(&d.info, &d.comment, &d.packet); ret < 0 {
				return errors.New("vorbis: unable to read the secondary Vorbis header")
			}
			headersRead++
		}
	}

	d.info.Deref()
	d.comment.Deref()
	d.comment.UserComments = make([][]byte, d.comment.Comments)
	d.comment.Deref()
	return nil
}

// Decode starts the decoding process until EOS occurred or a stop signal will be received.
func (d *Decoder) Decode() error {
	d.Lock()
	defer d.Unlock()
	if d.closed {
		return errors.New("decoder: decoder has already been closed")
	}

	if ret := vorbis.SynthesisInit(&d.dspState, &d.info); ret < 0 {
		err := errors.New("vorbis: error during playback initialization")
		d.reportError(err)
		return err
	}
	defer vorbis.DspClear(&d.dspState)

	vorbis.BlockInit(&d.dspState, &d.block)
	defer vorbis.BlockClear(&d.block)

	frame := make([][]float32, 0, d.samplesPerChannel)
	pcm := [][][]float32{
		make([][]float32, d.info.Channels),
	}

	defer func() {
		if len(frame) > 0 {
			d.sendFrame(frame)
		}
	}()

	for !d.stopRequested() {
		n, err := d.readNextPage(&frame, pcm)
		switch err {
		case nil:
			continue
		case io.EOF:
			if n > 0 {
				// has some data on the last page
				continue
			}
			return nil
		default:
			// a fatal error occured
			d.reportError(err)
			return err
		}
	}
	return nil
}

func (d *Decoder) sendFrame(frame [][]float32) {
	select {
	case <-d.stopChan:
		return
	case d.pcmOut <- frame:
	}
}

func (d *Decoder) readNextPage(frame *[][]float32, pcm [][][]float32) (n int, err error) {
	if ret := vorbis.OggSyncPageout(&d.syncState, &d.page); ret < 0 {
		d.onError(errors.New("vorbis: corrupt or missing data in bitstream"))
		return 0, nil // non-fatal
	} else if ret == 0 {
		// need more data
		return d.readChunk(d.input)
	}

	// page is synced at this point
	vorbis.OggStreamPagein(&d.streamState, &d.page)

	for !d.stopRequested() {
		if ret := vorbis.OggStreamPacketout(&d.streamState, &d.packet); ret < 0 {
			continue // skip packet
		} else if ret == 0 {
			// no packets left on the page, go to the new one
			return 0, nil
		}
		if vorbis.Synthesis(&d.block, &d.packet) == 0 {
			vorbis.SynthesisBlockin(&d.dspState, &d.block)
		}
		samples := vorbis.SynthesisPcmout(&d.dspState, pcm)
		for ; samples > 0; samples = vorbis.SynthesisPcmout(&d.dspState, pcm) {
			space := int32(d.samplesPerChannel - len(*frame))
			if samples > space {
				samples = space
			}
			for i := 0; i < int(samples); i++ {
				sample := make([]float32, d.info.Channels)
				for j := 0; j < int(d.info.Channels); j++ {
					sample[j] = pcm[0][j][:samples][i]
				}
				*frame = append(*frame, sample)
			}
			if len(*frame) == d.samplesPerChannel {
				d.sendFrame(*frame)
				*frame = make([][]float32, 0, d.samplesPerChannel)
			}
			vorbis.SynthesisRead(&d.dspState, samples)
		}
	}
	if d.stopRequested() || vorbis.OggPageEos(&d.page) == 1 {
		return 0, io.EOF
	}
	return 0, nil
}
