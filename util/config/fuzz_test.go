package config

import "testing"

func FuzzLoadDataNeverPanics(f *testing.F) {
	f.Add([]byte(`{"enabled":true}`), uint8(0))
	f.Add([]byte("name: value\n"), uint8(1))
	f.Add([]byte("name = 'value'\n"), uint8(2))
	f.Add([]byte("NAME=value\n"), uint8(3))
	formats := [...]string{".json", ".yaml", ".toml", ".env"}
	f.Fuzz(func(t *testing.T, data []byte, formatIndex uint8) {
		config := New()
		_ = config.loadData(data, formats[int(formatIndex)%len(formats)])
	})
}
