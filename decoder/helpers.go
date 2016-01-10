package decoder

func toString(buf []byte, maxlen int) string {
	buf = buf[:maxlen]
	for i := range buf {
		if buf[i] == 0 {
			return string(buf[:i:i])
		}
	}
	// not nul-terminated or len > maxlen
	return ""
}
