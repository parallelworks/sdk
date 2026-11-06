package parallelworks

import (
	"bytes"
	"encoding/json"
)

// Older platform releases return each executed job as stored: a single need as a string, an if:
// still to evaluate as its expression, and steps that are not objects. These decoders accept that
// shape so a newer client still reads runs from them. What else those releases return, such as a
// step's run and outputs, no field reads.

func (j *ExecutedJob) UnmarshalJSON(data []byte) error {
	type plain ExecutedJob
	raw := struct {
		*plain
		Needs json.RawMessage `json:"needs,omitempty"`
		If    json.RawMessage `json:"if,omitempty"`
	}{plain: (*plain)(j)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	j.Needs = lenientStrings(raw.Needs)
	j.If = nil
	var resolved *bool
	if json.Unmarshal(raw.If, &resolved) == nil {
		j.If = resolved
	}
	return nil
}

func (s *ExecutedStep) UnmarshalJSON(data []byte) error {
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		*s = ExecutedStep{}
		return nil
	}
	type plain ExecutedStep
	return json.Unmarshal(data, (*plain)(s))
}

func lenientStrings(data json.RawMessage) []string {
	var one string
	if json.Unmarshal(data, &one) == nil {
		return []string{one}
	}
	var many []string
	if json.Unmarshal(data, &many) == nil {
		return many
	}
	return nil
}
