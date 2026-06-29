package anthropic_test

import (
	"testing"

	"github.com/liushuangls/go-anthropic/v2"
)

func TestCurrentToolVersionConstructors(t *testing.T) {
	t.Run("text editor 20250728", func(t *testing.T) {
		def := anthropic.NewTextEditorToolDefinition20250728()
		if def.Type != "text_editor_20250728" {
			t.Fatalf("expected type text_editor_20250728, got %q", def.Type)
		}
		if def.Name != "str_replace_based_edit_tool" {
			t.Fatalf("expected name str_replace_based_edit_tool, got %q", def.Name)
		}
	})

	t.Run("bash 20250124", func(t *testing.T) {
		def := anthropic.NewBashToolDefinition20250124("bash")
		if def.Type != "bash_20250124" {
			t.Fatalf("expected type bash_20250124, got %q", def.Type)
		}
		if def.Name != "bash" {
			t.Fatalf("expected name bash, got %q", def.Name)
		}
	})

	t.Run("computer use 20250124", func(t *testing.T) {
		def := anthropic.NewComputerUseToolDefinition20250124("computer", 1024, 768, nil)
		if def.Type != "computer_20250124" {
			t.Fatalf("expected type computer_20250124, got %q", def.Type)
		}
		if def.Name != "computer" {
			t.Fatalf("expected name computer, got %q", def.Name)
		}
		if def.DisplayWidthPx != 1024 || def.DisplayHeightPx != 768 {
			t.Fatalf("unexpected display dimensions: %dx%d", def.DisplayWidthPx, def.DisplayHeightPx)
		}
	})
}
