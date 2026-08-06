package parallelworks

import (
	"io"
	"strings"
	"testing"
)

func TestSSEReaderDone(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "terminator", body: "data: [DONE]\n\n", want: true},
		{name: "transport eof", body: "", want: false},
		{name: "chunk then transport eof", body: `data: {"choices":[]}` + "\n\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewSSEReader(strings.NewReader(tt.body))
			for {
				_, err := reader.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := reader.Done(); got != tt.want {
				t.Fatalf("Done() = %v, want %v", got, tt.want)
			}
		})
	}
}
