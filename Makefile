all:
	cgogen vorbis.yml

clean:
	rm -f vorbis/cgo_helpers.go vorbis/cgo_helpers.h vorbis/doc.go vorbis/types.go
	rm -f vorbis/vorbis.go

test:
	cd vorbis && go build