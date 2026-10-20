package parallelworks

import (
	"bytes"
	"encoding/json"
)

// Older platform releases return each executed job as stored: outputs as a JSON object in a
// string, a single need as a string, ssh values as numbers, and steps that are not objects.
// These decoders accept that shape so a newer client still reads runs from them.

func (j *ExecutedJob) UnmarshalJSON(data []byte) error {
	type plain ExecutedJob
	raw := struct {
		*plain
		Needs   json.RawMessage `json:"needs,omitempty"`
		Outputs json.RawMessage `json:"outputs,omitempty"`
		SSH     json.RawMessage `json:"ssh,omitempty"`
	}{plain: (*plain)(j)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	j.Needs = lenientStrings(raw.Needs)
	j.Outputs = lenientStringMap(raw.Outputs)
	j.SSH = lenientStringMap(raw.SSH)
	return nil
}

func (s *ExecutedStep) UnmarshalJSON(data []byte) error {
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		*s = ExecutedStep{}
		return nil
	}
	type plain ExecutedStep
	raw := struct {
		*plain
		Outputs json.RawMessage `json:"outputs,omitempty"`
	}{plain: (*plain)(s)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Outputs = lenientStringMap(raw.Outputs)
	return nil
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

// lenientStringMap reads an object, or an object encoded in a string, coercing each value to
// its text: numbers as written, null as "", and objects and arrays as their JSON.
func lenientStringMap(data json.RawMessage) map[string]string {
	var encoded string
	if json.Unmarshal(data, &encoded) == nil {
		data = json.RawMessage(encoded)
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(data, &values) != nil || values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		var s string
		switch {
		case json.Unmarshal(v, &s) == nil:
			out[k] = s
		case string(v) == "null":
			out[k] = ""
		default:
			out[k] = string(v)
		}
	}
	return out
}
