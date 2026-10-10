package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestParseRESPFraming(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:    "valid bulk string",
			input:   "*1\r\n$2\r\nhi\r\n",
			want:    []string{"hi"},
			wantErr: false,
		},
		{
			name:    "missing bulk string terminator",
			input:   "*1\r\n$2\r\nhiXX",
			wantErr: true,
		},
		{
			name:    "array header missing carriage return",
			input:   "*1\n$2\r\nhi\r\n",
			wantErr: true,
		},
		{
			name:    "bulk length header missing carriage return",
			input:   "*1\r\n$2\nhi\r\n",
			wantErr: true,
		},
		{
			name:    "truncated bulk string",
			input:   "*1\r\n$3\r\nhi",
			wantErr: true,
		},
		{
			name:    "valid empty bulk string",
			input:   "*1\r\n$0\r\n\r\n",
			want:    []string{""},
			wantErr: false,
		},
		{
			name:    "invalid array length",
			input:   "*abc\r\n",
			wantErr: true,
		},
		{
			name:    "array length with leading whitespace",
			input:   "* 1\r\n$2\r\nhi\r\n",
			wantErr: true,
		},
		{
			name:    "array length with trailing whitespace",
			input:   "*1 \r\n$2\r\nhi\r\n",
			wantErr: true,
		},
		{
			name:    "invalid bulk length",
			input:   "*1\r\n$abc\r\n",
			wantErr: true,
		},
		{
			name:    "bulk length with leading whitespace",
			input:   "*1\r\n$ 2\r\nhi\r\n",
			wantErr: true,
		},
		{
			name:    "bulk length with trailing whitespace",
			input:   "*1\r\n$2 \r\nhi\r\n",
			wantErr: true,
		},
		{
			name:    "invalid payload terminator",
			input:   "*1\r\n$2\r\nhi\rX",
			wantErr: true,
		},
		{
			name:    "negative array length",
			input:   "*-1\r\n",
			wantErr: true,
		},
		{
			name:    "negative bulk length",
			input:   "*1\r\n$-1\r\n",
			wantErr: true,
		},
		{
			name:    "empty array",
			input:   "*0\r\n",
			want:    []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))

			got, err := parseRESP(reader)

			if (err != nil) != tt.wantErr {
				t.Fatalf(
					"parseRESP() error = %v, wantErr %v",
					err,
					tt.wantErr,
				)
			}

			if err != nil {
				return
			}

			if len(got) != len(tt.want) {
				t.Fatalf(
					"parseRESP() returned %d arguments, want %d: %#v",
					len(got),
					len(tt.want),
					got,
				)
			}

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf(
						"argument %d = %q, want %q",
						i,
						got[i],
						tt.want[i],
					)
				}
			}
		})
	}
}
