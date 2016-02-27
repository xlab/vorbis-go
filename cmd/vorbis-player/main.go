package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"unsafe"

	"github.com/apcera/termtables"
	"github.com/jawher/mow.cli"
	"github.com/xlab/closer"
	"github.com/xlab/portaudio-go/portaudio"
	"github.com/xlab/vorbis-go/decoder"
)

const (
	samplesPerChannel = 2048
	bitDepth          = 16
	sampleFormat      = portaudio.PaFloat32
)

var (
	app = cli.App("vorbis-player", "A player implemented in Go that can read OggVorbis files and play using PortAudio.")
	uri = app.StringArg("URI", "", "A local .ogg Vorbis file or an URL pointing to file.")
)

func main() {
	log.SetFlags(0)
	app.Action = appRun
	app.Run(os.Args)
}

func appRun() {
	defer closer.Close()
	closer.Bind(func() {
		log.Println("Bye!")
	})

	if err := portaudio.Initialize(); paError(err) {
		log.Fatalln("PortAudio init error:", paErrorText(err))
	}
	closer.Bind(func() {
		if err := portaudio.Terminate(); paError(err) {
			log.Println("PortAudio term error:", paErrorText(err))
		}
	})

	var input io.Reader
	if strings.HasPrefix(*uri, "http://") || strings.HasPrefix(*uri, "https://") {
		resp, err := http.Get(*uri)
		if err != nil {
			log.Fatalln(err)
		}
		closer.Bind(func() {
			resp.Body.Close()
		})
		input = resp.Body
	} else {
		f, err := os.Open(*uri)
		if err != nil {
			log.Fatalln(err)
		}
		closer.Bind(func() {
			f.Close()
		})
		input = f
	}

	dec, err := decoder.New(input, samplesPerChannel)
	if err != nil {
		log.Fatalln(err)
	}
	closer.Bind(dec.Close)

	info := dec.Info()
	log.Println(fileInfoTable(info))

	dec.SetErrorHandler(func(err error) {
		log.Println("[WARN]", err)
	})
	go func() {
		dec.Decode()
		dec.Close()
	}()

	var wg sync.WaitGroup
	var stream *portaudio.Stream
	callback := paCallback(&wg, int(info.Channels), dec.SamplesOut())
	if err := portaudio.OpenDefaultStream(&stream, 0, int32(info.Channels), sampleFormat, info.SampleRate,
		samplesPerChannel, callback, nil); paError(err) {
		log.Fatalln("PortAudio error:", paErrorText(err))
	}
	closer.Bind(func() {
		if err := portaudio.CloseStream(stream); paError(err) {
			log.Println("[WARN] PortAudio error:", paErrorText(err))
		}
	})

	if err := portaudio.StartStream(stream); paError(err) {
		log.Fatalln("PortAudio error:", paErrorText(err))
	}
	closer.Bind(func() {
		if err := portaudio.StopStream(stream); paError(err) {
			log.Fatalln("[WARN] PortAudio error:", paErrorText(err))
		}
	})

	log.Println("Playing...")
	wg.Wait()
}

func fileInfoTable(info decoder.Info) string {
	table := termtables.CreateTable()
	table.UTF8Box()
	table.AddTitle("FILE INFO")
	for _, comment := range info.Comments {
		parts := strings.Split(comment, "=")
		if row := table.AddRow(parts[0]); len(parts) > 1 {
			row.AddCell(parts[1])
		}
	}
	if len(info.Comments) > 0 {
		table.AddSeparator()
	}
	table.AddRow("Bitstream", fmt.Sprintf("%d channel, %.1fHz", info.Channels, info.SampleRate))
	table.AddRow("Encoded by", info.Vendor)
	return table.Render()
}

func paCallback(wg *sync.WaitGroup, channels int, samples <-chan [][]float32) portaudio.StreamCallback {
	wg.Add(1)
	return func(_ unsafe.Pointer, output unsafe.Pointer, sampleCount uint,
		_ *portaudio.StreamCallbackTimeInfo, _ portaudio.StreamCallbackFlags, _ unsafe.Pointer) int32 {

		const (
			statusContinue = int32(portaudio.PaContinue)
			statusComplete = int32(portaudio.PaComplete)
		)

		frame, ok := <-samples
		if !ok {
			wg.Done()
			return statusComplete
		}
		if len(frame) > int(sampleCount) {
			frame = frame[:sampleCount]
		}

		var idx int
		out := (*(*[1 << 32]float32)(unsafe.Pointer(output)))[:int(sampleCount)*channels]
		for _, sample := range frame {
			if len(sample) > channels {
				sample = sample[:channels]
			}
			for i := range sample {
				out[idx] = sample[i]
				idx++
			}
		}

		return statusContinue
	}
}
