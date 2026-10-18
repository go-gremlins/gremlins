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

		d, err := NewWithCmd(m.call, "root")

		if d != nil && err != nil {
			t.Fatal("incorrect result")
		}
	})

	t.Run("must return error", func(t *testing.T) {
		viper.Set(configuration.UnleashDiffRef, "test")

		m := &mock{
			outputErr: errors.New("test"),
		}

		_, err := NewWithCmd(m.call, "root")
		if err == nil {
			t.Error("must return error")
		}

		if m.calls != 1 {
			t.Fatal("cmd not called")
		}

		expectedArgs := []string{"-C", "root", "diff", "--merge-base", "--relative", "test"}

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

		_, err := NewWithCmd(m.call, "root")
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

		result, err := NewWithCmd(m.call, "root")

		if err != nil || !reflect.DeepEqual(result, expected) {
			t.Log("err", err)
			t.Log("result", result)
			t.Error("unexpected result")
		}
	})
}

func TestEveryAddedLineOfAFragmentIsAChange(t *testing.T) {
	viper.Set(configuration.UnleashDiffRef, "test")
	defer viper.Set(configuration.UnleashDiffRef, "")

	result, err := NewWithCmd((&mock{output: []byte(testGappedDiff)}).call, "root")
	if err != nil {
		t.Fatal(err)
	}

	expected := Diff{
		"test/test": {{StartLine: 42, EndLine: 42}, {StartLine: 46, EndLine: 46}},
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
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
	// testGappedDiff has one fragment whose two added lines are separated by
	// context, so a range derived from the fragment's first added line and
	// its added count would cover line 42 only and miss line 46.
	testGappedDiff = `
diff --git a/test/test b/test/test
index 54051bc..b92c425 100644
--- a/test/test
+++ b/test/test
@@ -41,8 +41,9 @@ const (
 	test = "test"
+	test = "added"
 	test = "test"
 	test = "test"
 	test = "test"
+	test = "added"
-	test = "removed"
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
