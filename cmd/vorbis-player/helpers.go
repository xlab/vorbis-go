package main

import "github.com/xlab/portaudio-go/portaudio"

func paError(err portaudio.Error) bool {
	return portaudio.ErrorCode(err) != portaudio.PaNoError

}

func paErrorText(err portaudio.Error) string {
	return portaudio.GetErrorText(err)
}
