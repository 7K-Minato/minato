package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	operatorv1 "github.com/7k-group/minato/api/operator/v1"
)

func TestValidateParams(t *testing.T) {
	r := &ActionExecutionReconciler{}

	decl := &operatorv1.ActionDecl{
		Name: "test-action",
		Params: map[string]operatorv1.ActionParamSchema{
			"required-param": {Type: "string", Required: true},
			"optional-param": {Type: "string", Required: false, Default: "default"},
		},
	}

	tests := []struct {
		name    string
		params  map[string]string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "all params present",
			params:  map[string]string{"required-param": "value", "optional-param": "opt"},
			wantErr: false,
		},
		{
			name:    "only required param",
			params:  map[string]string{"required-param": "value"},
			wantErr: false,
		},
		{
			name:    "missing required param",
			params:  map[string]string{"optional-param": "opt"},
			wantErr: true,
			errMsg:  "required param \"required-param\" missing",
		},
		{
			name:    "no params",
			params:  map[string]string{},
			wantErr: true,
			errMsg:  "required param \"required-param\" missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &operatorv1.ActionExecution{
				Spec: operatorv1.ActionExecutionSpec{
					Params: tt.params,
				},
			}
			err := r.validateParams(exec, decl)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
