package diff

import (
	"errors"
	"reflect"
	"testing"

	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/internal/configuration"
)

func TestNewWithCmd(t *testing.T) {
	t.Run("must return nil on empty flag", func(t *testing.T) {
		m := &mock{}

		d, err := NewWithCmd(m.call)

		if d != nil && err != nil {
			t.Fatal("incorrect result")
		}
	})

	t.Run("must return error", func(t *testing.T) {
		viper.Set(configuration.UnleashDiffRef, "test")

		m := &mock{
			outputErr: errors.New("test"),
		}

		_, err := NewWithCmd(m.call)
		if err == nil {
			t.Error("must return error")
		}

		if m.calls != 1 {
			t.Fatal("cmd not called")
		}

		expectedArgs := []string{"diff", "--merge-base", "test"}

		if m.callName != "git" || !reflect.DeepEqual(m.callArgs, expectedArgs) {
			t.Log("name", m.callName)
			t.Log("args", m.callArgs)
			t.Error("cmd not called properly")
		}
	})

	t.Run("must return diff error", func(t *testing.T) {
		viper.Set(configuration.UnleashDiffRef, "test")

		m := &mock{
			output: []byte(testErrDiff),
		}

		_, err := NewWithCmd(m.call)
		if err == nil {
			t.Error("must return error")
		}
	})

	t.Run("must return changes", func(t *testing.T) {
		viper.Set(configuration.UnleashDiffRef, "test")

		m := &mock{
			output: []byte(testDiff),
		}

		expected := Diff{
			"test/test": {{StartLine: 44, EndLine: 44}},
		}

		result, err := NewWithCmd(m.call)

		if err != nil || !reflect.DeepEqual(result, expected) {
			t.Log("err", err)
			t.Log("result", result)
			t.Error("unexpected result")
		}
	})
}

type mock struct {
	calls     int
	callName  string
	callArgs  []string
	output    []byte
	outputErr error
}

func (m *mock) call(name string, args ...string) execCmd {
	m.calls++
	m.callName = name
	m.callArgs = args

	return m
}

func (m *mock) CombinedOutput() ([]byte, error) {
	return m.output, m.outputErr
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
+	test = "test"
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
+	test = "test"
 	test = "test"
 )
`
)
