package diff

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/internal/configuration"
)

func TestNewWithCmd_EmptyDiffRef(t *testing.T) {
	m := &mock{}

	d, err := NewWithCmd(m.call, "/repo", "")

	if !reflect.DeepEqual(d, Diff{}) || err != nil {
		t.Fatal("incorrect result")
	}
}

func TestNewWithCmd_Errors(t *testing.T) {
	tests := []struct {
		name      string
		responses []mockResponse
		wantCalls int
		wantArgs  []string
	}{
		{
			name:      "git rev-parse failure",
			responses: []mockResponse{{err: errors.New("not a git repo")}},
		},
		{
			name: "git diff failure",
			responses: []mockResponse{
				{output: []byte("/repo\n")},
				{err: errors.New("test")},
			},
			wantCalls: 2,
			wantArgs:  []string{"diff", "--merge-base", "test"},
		},
		{
			name: "diff parse error",
			responses: []mockResponse{
				{output: []byte("/repo\n")},
				{output: []byte(testErrDiff)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Set(configuration.UnleashDiffRef, "test")
			defer viper.Reset()

			m := &mock{responses: tt.responses}

			_, err := NewWithCmd(m.call, "/repo", "")
			if err == nil {
				t.Error("must return error")
			}

			if tt.wantCalls > 0 && len(m.calls) != tt.wantCalls {
				t.Fatalf("expected %d cmd calls, got %d", tt.wantCalls, len(m.calls))
			}

			if tt.wantArgs != nil {
				if m.calls[1].name != "git" || !reflect.DeepEqual(m.calls[1].args, tt.wantArgs) {
					t.Errorf("cmd not called properly: name=%s args=%v", m.calls[1].name, m.calls[1].args)
				}
			}
		})
	}
}

func TestNewWithCmd_FlatRepo(t *testing.T) {
	viper.Set(configuration.UnleashDiffRef, "test")
	defer viper.Reset()

	m := &mock{
		responses: []mockResponse{
			{output: []byte("/repo\n")},
			{output: []byte(testDiff)},
		},
	}

	expected := Diff{
		changes: map[FileName][]Change{
			"test/test": {{StartLine: 44, EndLine: 44}},
		},
		moduleRel: ".",
	}

	result, err := NewWithCmd(m.call, "/repo", "")

	if err != nil || !reflect.DeepEqual(result, expected) {
		t.Log("err", err)
		t.Log("result", result)
		t.Error("unexpected result")
	}
}

func TestNewWithCmd_Monorepo(t *testing.T) {
	viper.Set(configuration.UnleashDiffRef, "main")
	defer viper.Reset()

	m := &mock{
		responses: []mockResponse{
			{output: []byte("/home/user/repo\n")},
			{output: []byte(testMonorepoDiff)},
		},
	}

	expected := Diff{
		changes: map[FileName][]Change{
			"service-a/main.go": {{StartLine: 44, EndLine: 44}},
		},
		moduleRel: "service-a",
	}

	result, err := NewWithCmd(m.call, "/home/user/repo/service-a", "")

	if err != nil || !reflect.DeepEqual(result, expected) {
		t.Log("err", err)
		t.Log("result", result)
		t.Error("unexpected result")
	}
}

func TestNewWithCmd_RelativeModuleRoot(t *testing.T) {
	viper.Set(configuration.UnleashDiffRef, "main")
	defer viper.Reset()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	repoRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "service-a"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatal(err)
	}

	m := &mock{
		responses: []mockResponse{
			{output: []byte(repoRoot + "\n")},
			{output: []byte(testMonorepoDiff)},
		},
	}

	expected := Diff{
		changes: map[FileName][]Change{
			"service-a/main.go": {{StartLine: 44, EndLine: 44}},
		},
		moduleRel: "service-a",
	}

	result, err := NewWithCmd(m.call, "service-a", "")

	if err != nil || !reflect.DeepEqual(result, expected) {
		t.Log("err", err)
		t.Log("result", result)
		t.Error("unexpected result")
	}
}

type mockResponse struct {
	output []byte
	err    error
}

type mockCall struct {
	name string
	args []string
}

type mock struct {
	calls     []mockCall
	responses []mockResponse
	idx       int
}

func (m *mock) call(name string, args ...string) execCmd {
	m.calls = append(m.calls, mockCall{name: name, args: args})

	return m
}

func (m *mock) CombinedOutput() ([]byte, error) {
	if m.idx >= len(m.responses) {
		return nil, nil
	}
	resp := m.responses[m.idx]
	m.idx++

	return resp.output, resp.err
}

const (
	testDiff = `
diff --git a/test/test b/test/test
index 54051bc..b92c425 100644
--- a/test/test
+++ b/test/test
@@ -41,6 +41,7 @@ const (
 test = "test"
 test = "test"
 test = "test"
+test = "test"
 test = "test"
 test = "test"
 )
`
	testErrDiff = `
diff --git a/test/test b/test/test
index 54051bc..b92c425 100644
--- a/test/test
+++ b/test/test
@@ -41,7 +41,7 @@ const (
 test = "test"
+test = "test"
 test = "test"
 )
`
	testMonorepoDiff = `
diff --git a/service-a/main.go b/service-a/main.go
index 54051bc..b92c425 100644
--- a/service-a/main.go
+++ b/service-a/main.go
@@ -41,6 +41,7 @@ const (
 test = "test"
 test = "test"
 test = "test"
+test = "test"
 test = "test"
 test = "test"
 )
`
)
