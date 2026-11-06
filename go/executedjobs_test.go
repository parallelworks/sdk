package parallelworks

import (
	"encoding/json"
	"testing"
)

// oldWorkflowRun is GET /api/workflow-runs/{slug} from a release that returned the stored jobs as they are.
const oldWorkflowRun = `{
  "id": "6720f0c2a1b2c3d4e5f60718",
  "user": "alice",
  "inlineRun": false,
  "marketplace": false,
  "executedJobs": {
    "preprocessing": {
      "needs": "setup",
      "if": "${{ needs.setup.outputs.ready }}",
      "env": {"TOKEN": "x"},
      "ssh": {"remoteHost": "10.0.0.5", "port": 22, "jump": null},
      "outputs": "{\"ready\":\"true\",\"count\":3}",
      "status": "error",
      "steps": [
        {"name": "Setup", "run": "./setup.sh", "status": "error", "retry": {"max-attempts": 2},
         "outputs": "{\"path\":\"/tmp/x\"}",
         "annotations": [{"type": "error", "message": "boom", "line": 12}]},
        "not a step",
        {"name": "Runner", "uses": "marketplace/runner/v1", "with": {"resource": {"ip": "10.0.0.5"}},
         "subworkflow": {"sessionInfo": {"slug": "s"}, "directory": "/tmp/runner",
           "jobs": {"inner": {"needs": ["a"], "outputs": {"url": "${{ steps.x.outputs.url }}"}, "status": "completed"}}}}
      ]
    },
    "gate": {"if": false, "status": "skipped", "_matrix": {"originaljob": "gate", "index": 0, "failfast": true}}
  }
}`

func TestWorkflowRunDecodesOldExecutedJobs(t *testing.T) {
	var run WorkflowRunDetailResponse
	if err := json.Unmarshal([]byte(oldWorkflowRun), &run); err != nil {
		t.Fatalf("decoding a run from an older release: %v", err)
	}

	pre := run.ExecutedJobs["preprocessing"]
	if got := pre.Needs; len(got) != 1 || got[0] != "setup" {
		t.Errorf("needs = %v, want [setup]", got)
	}
	if pre.If != nil {
		t.Errorf("if = %v, want none for an if: still to evaluate", *pre.If)
	}
	if len(pre.Steps) != 3 {
		t.Fatalf("steps = %d, want 3 with the entry that is not a step kept at its index", len(pre.Steps))
	}
	if got := pre.Steps[0]; len(got.Annotations) != 1 || got.Annotations[0].Message != "boom" {
		t.Errorf("first step = %+v", got)
	}
	if got := pre.Steps[1]; got.Name != nil || got.Status != nil {
		t.Errorf("non-step entry = %+v, want empty", got)
	}
	sub := pre.Steps[2].Subworkflow
	if sub == nil {
		t.Fatal("subworkflow missing")
	}
	inner := sub.Jobs["inner"]
	if inner.Status == nil || *inner.Status != "completed" || len(inner.Needs) != 1 {
		t.Errorf("inner job = %+v", inner)
	}

	gate := run.ExecutedJobs["gate"]
	if gate.If == nil || *gate.If {
		t.Errorf("if = %v, want false", gate.If)
	}
	if gate.Matrix == nil || gate.Matrix.Originaljob != "gate" {
		t.Errorf("matrix = %+v", gate.Matrix)
	}
}

func TestWorkflowRunDecodesTypedExecutedJobs(t *testing.T) {
	var run WorkflowRunDetailResponse
	body := `{"id":"x","user":"a","inlineRun":false,"marketplace":false,
	  "executedJobs":{"build":{"needs":["setup"],"if":true,"steps":[{"name":"s","uses":"parallelworks/checkout"}]},"skip":{"if":null}}}`
	if err := json.Unmarshal([]byte(body), &run); err != nil {
		t.Fatal(err)
	}
	build := run.ExecutedJobs["build"]
	if build.Needs[0] != "setup" || build.If == nil || !*build.If || *build.Steps[0].Name != "s" || *build.Steps[0].Uses != "parallelworks/checkout" {
		t.Errorf("build = %+v", build)
	}
	if skip := run.ExecutedJobs["skip"]; skip.If != nil {
		t.Errorf("if = %v, want none for null", *skip.If)
	}
}
