//go:build linux
// +build linux

package dmesg

import (
	"errors"
	"io"
	"testing"
	"time"
)

type fakeSeeker struct {
	offset int64
	whence int
	err    error
}

func (f *fakeSeeker) Seek(offset int64, whence int) (int64, error) {
	f.offset = offset
	f.whence = whence
	return 0, f.err
}

func TestParseStartupLookback(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Duration
		wantAll bool
		wantErr bool
	}{
		{name: "empty", value: "", want: 0},
		{name: "zero", value: "0", want: 0},
		{name: "zero duration", value: "0s", want: 0},
		{name: "duration", value: "1h", want: time.Hour},
		{name: "read all", value: "-1", wantAll: true},
		{name: "invalid duration", value: "1hour", wantErr: true},
		{name: "negative duration", value: "-1h", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotAll, err := parseStartupLookback(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseStartupLookback(%q) expected error", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseStartupLookback(%q) error = %v", tt.value, err)
			}
			if got != tt.want {
				t.Fatalf("parseStartupLookback(%q) duration = %v, want %v", tt.value, got, tt.want)
			}
			if gotAll != tt.wantAll {
				t.Fatalf("parseStartupLookback(%q) readAll = %v, want %v", tt.value, gotAll, tt.wantAll)
			}
		})
	}
}

func TestSetupStartupPosition(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		nowUsec    int64
		wantWhence int
		wantMin    int64
	}{
		{
			name:       "default seeks to end",
			value:      "",
			wantWhence: io.SeekEnd,
			wantMin:    0,
		},
		{
			name:       "zero duration seeks to end",
			value:      "0s",
			wantWhence: io.SeekEnd,
			wantMin:    0,
		},
		{
			name:       "read all seeks to beginning",
			value:      "-1",
			wantWhence: io.SeekStart,
			wantMin:    0,
		},
		{
			name:       "lookback seeks to beginning and returns cutoff",
			value:      "1h",
			nowUsec:    int64((2 * time.Hour).Microseconds()),
			wantWhence: io.SeekStart,
			wantMin:    int64(time.Hour.Microseconds()),
		},
		{
			name:       "lookback cutoff is clamped to zero",
			value:      "3h",
			nowUsec:    int64(time.Hour.Microseconds()),
			wantWhence: io.SeekStart,
			wantMin:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seeker := &fakeSeeker{}
			got, err := setupStartupPosition(seeker, tt.value, func() (int64, error) {
				return tt.nowUsec, nil
			})
			if err != nil {
				t.Fatalf("setupStartupPosition(%q) error = %v", tt.value, err)
			}
			if seeker.offset != 0 {
				t.Fatalf("seek offset = %d, want 0", seeker.offset)
			}
			if seeker.whence != tt.wantWhence {
				t.Fatalf("seek whence = %d, want %d", seeker.whence, tt.wantWhence)
			}
			if got != tt.wantMin {
				t.Fatalf("min timestamp = %d, want %d", got, tt.wantMin)
			}
		})
	}
}

func TestSetupStartupPositionReturnsSeekError(t *testing.T) {
	seeker := &fakeSeeker{err: errors.New("seek failed")}
	_, err := setupStartupPosition(seeker, "0s", func() (int64, error) {
		return 0, nil
	})
	if err == nil {
		t.Fatal("setupStartupPosition expected error")
	}
}

func TestSetupStartupPositionReturnsClockError(t *testing.T) {
	seeker := &fakeSeeker{}
	_, err := setupStartupPosition(seeker, "1h", func() (int64, error) {
		return 0, errors.New("clock failed")
	})
	if err == nil {
		t.Fatal("setupStartupPosition expected error")
	}
}
