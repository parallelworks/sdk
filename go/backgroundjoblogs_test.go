package parallelworks

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBackgroundJobLogStreamResumesAndDecodesFinalEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" || r.URL.Query().Get("offset") != "3" || r.URL.Query().Get("afterRun") != "1" {
			t.Errorf("stream request = %s, accept=%s", r.URL, r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"log\":\"def\",\"offset\":3,\"nextOffset\":6,\"size\":6,\"run\":\"1\",\"state\":\"running\",\"done\":false,\"runs\":[]}\n\n")
		_, _ = io.WriteString(w, "data: {\"log\":\"\",\"offset\":6,\"nextOffset\":6,\"size\":6,\"run\":\"1\",\"state\":\"completed\",\"done\":true,\"runs\":[]}\n\n")
	}))
	defer server.Close()
	offset, run := int64(3), "1"
	stream, err := NewClient(server.URL).StreamBackgroundJobLogsStream(t.Context(), "job", StreamBackgroundJobLogsParams{Offset: &offset, AfterRun: &run})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	first, err := stream.Next()
	if err != nil || first.Log != "def" || first.Offset != 3 || first.NextOffset != 6 || first.Done {
		t.Fatalf("first event = %+v, %v", first, err)
	}
	last, err := stream.Next()
	if err != nil || !last.Done || last.State != "completed" || last.NextOffset != 6 {
		t.Fatalf("final event = %+v, %v", last, err)
	}
	if _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("end of stream = %v", err)
	}
}
