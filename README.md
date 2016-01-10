vorbis-go ![vorbis](https://xiph.org/images/logos/fish_xiph_org.png)
=========

The package provides Go bindings for OggVorbis encoder/decoder reference implementation from [Xiph.org](https://www.xiph.org).<br />
All the binding code has automatically been generated with rules defined in [vorbis.yml](/vorbis.yml).

### Usage

```
$ go get github.com/xlab/vorbis-go/vorbis

$ go get github.com/xlab/vorbis-go/decoder
(optionally if you need a quickstart in decoding)
```

Examples of usage: an implemented [OggVorbis decoder](/decoder) for Go programming language.

### Demo

There is a player implemented in Go that can read OggVorbis files and play them via [portaudio-go](https://github.com/xlab/portaudio-go). So you will need to get portaudio installed first.

```
$ brew install portaudio vorbis ogg
$ go get github.com/xlab/vorbis-go/cmd/vorbis-player

$ vorbis-player http://dl.xlab.is/music/ogg/cloud_passade_3.ogg
╭─────────────────────────────────────────────────────────────╮
│                          FILE INFO                          │
├─────────────┬───────────────────────────────────────────────┤
│ TITLE       │ Cloud Passade No. 3                           │
│ ARTIST      │ Lubomyr Melnyk                                │
│ DATE        │ 2013                                          │
│ COMMENT     │ Visit http://unseenworlds.bandcamp.com        │
│ ALBUM       │ Three Solo Pieces                             │
│ TRACKNUMBER │ 3                                             │
│ ALBUMARTIST │ Lubomyr Melnyk                                │
│ ISRC        │ USVML1311003                                  │
├─────────────┼───────────────────────────────────────────────┤
│ Bitstream   │ 2 channel, 44100.0Hz                          │
│ Encoded by  │ Xiph.Org libVorbis I 20140122 (Turpakäräjiin) │
╰─────────────┴───────────────────────────────────────────────╯
Playing...
```

### Rebuilding the package

You will need to get the [cgogen](https://git.io/cgogen) tool installed first.

```
$ git clone https://github.com/xlab/vorbis-go && cd vorbis-go
$ make clean
$ make
```
