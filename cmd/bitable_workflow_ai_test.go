package cmd

import (
	"encoding/json"
	"testing"
)

func TestValidateWorkflowAISteps(t *testing.T) {
	bad := []string{
		`{"steps":[{"type":"AIAnalysisAction","data":{"identity_type":"bot"}}]}`,
		`{"steps":[{"type":"AIAnalysisAction","data":{"analysis_table_names":"t"}}]}`,
		`{"steps":[{"type":"AIClassificationBranch","data":{"mode":"Multi","classes":[]}}]}`,
		`{"steps":[{"type":"AIClassificationBranch","data":{"classes":[{"name":"a","desc":""}]}}]}`,
		`{"steps":[{"type":"AIClassificationBranch","data":{"classes":[{"name":"a","desc":""},{"name":"a","desc":""}]}}]}`,
	}
	for _, raw := range bad {
		var body map[string]any
		_ = json.Unmarshal([]byte(raw), &body)
		if err := validateWorkflowAISteps(body); err == nil {
			t.Errorf("应报错: %s", raw)
		}
	}
	var ok map[string]any
	_ = json.Unmarshal([]byte(`{"steps":[{"type":"AIClassificationBranch","data":{"classes":[{"name":"a","desc":""},{"name":"b","desc":"x"}]}},{"type":"AIAnalysisAction","data":{"identity_type":"maker","analysis_table_names":["t"]}}]}`), &ok)
	if err := validateWorkflowAISteps(ok); err != nil {
		t.Errorf("合法定义应通过: %v", err)
	}
}
