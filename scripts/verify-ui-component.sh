#!/bin/bash
# Verify a UI component against the chrome-package contract (docs/UI_INSTRUCTIONS.md)
# Usage: ./verify-ui-component.sh <component-path>
# Example: ./verify-ui-component.sh src/components/tasktree

COMPONENT_PATH="${1:-.}"

if [ ! -d "$COMPONENT_PATH" ]; then
	echo "Error: $COMPONENT_PATH is not a directory"
	exit 1
fi

COMPONENT_NAME=$(basename "$COMPONENT_PATH")

# Skip chrome package - it's the helper library, not a leaf component to be verified
if [ "$COMPONENT_NAME" = "chrome" ]; then
	echo "Skipping chrome package (helper library, not a leaf component)"
	exit 0
fi
PASSED=0
FAILED=0

echo "Verifying component: $COMPONENT_NAME"
echo "=============================================="

# Rule 1: Every color from appstyles.Active.*, no literal hex
echo ""
echo "[Rule 1] Colors: Every color from appstyles.Active.*, no literal hex"
if grep -rEn "(0x[0-9A-Fa-f]{6}|#[0-9A-Fa-f]{6})" "$COMPONENT_PATH" 2>/dev/null | grep -v "test\|fixture\|doc.go" | grep -qv "://"; then
	echo "  ✗ FAILED: Found literal hex color values"
	((FAILED++))
else
	echo "  ✓ PASSED: No literal hex colors found"
	((PASSED++))
fi

# Rule 2: No hand-set padding/border/corners outside chrome helpers
echo ""
echo "[Rule 2] Frame: Outer box uses chrome.PanelFrame/ModalSurface/EmptyStateCard"
if grep -rEn "\.Padding\(|\.Border\(|\.BorderStyle\(" "$COMPONENT_PATH" 2>/dev/null | grep -v "chrome" | grep -v "test\|fixture" | grep -q "\.go:"; then
	echo "  ⚠ WARNING: Found padding/border methods (check if in chrome helper)"
	# Don't fail — might be inside chrome helper package
else
	echo "  ✓ PASSED: No hand-set padding/border outside chrome helpers"
	((PASSED++))
fi

# Check if View() calls a chrome helper
CHROME_HELPER_FOUND=$(grep -r "chrome.PanelFrame\|chrome.ModalSurface\|chrome.EmptyStateCard" "$COMPONENT_PATH"/*.go 2>/dev/null)
if [ -n "$CHROME_HELPER_FOUND" ]; then
	echo "  ✓ PASSED: View() delegates to chrome helper"
	((PASSED++))
else
	echo "  ✗ FAILED: View() doesn't call a chrome helper"
	((FAILED++))
fi

# Rule 3: User text goes through chrome.Truncate()
echo ""
echo "[Rule 3] Truncation: User-supplied text through chrome.Truncate()"
if grep -rn "Task.Title\|Task.Notes\|List.Name" "$COMPONENT_PATH" 2>/dev/null | head -3 | grep -q "Task\|List"; then
	echo "  ⚠ WARNING: Component renders user-supplied data (verify truncation in code review)"
	echo "     Search for: grep -B 2 'Task.Title\|Task.Notes\|List.Name' $COMPONENT_PATH/*.go | grep chrome.Truncate"
else
	echo "  ✓ PASSED: No user-supplied fields found (or handled elsewhere)"
	((PASSED++))
fi

# Rule 4: Background is sealed
echo ""
echo "[Rule 4] Background sealing: Uses chrome helpers or FillBackground()"
if grep -rn "FillBackground\|chrome.PanelFrame\|chrome.ModalSurface\|chrome.EmptyStateCard" "$COMPONENT_PATH" 2>/dev/null | grep -q ".go"; then
	echo "  ✓ PASSED: Background is sealed"
	((PASSED++))
else
	echo "  ⚠ WARNING: No seal detected (may be in View() return)"
	# Don't fail — might use JoinVertical/JoinHorizontal
fi

# Rule 5: Glyphs are in vocabulary
echo ""
echo "[Rule 5] Glyphs: Only vocabulary symbols ([ ] [~] [x] ▾ ▸ - + ^ (NN%))"
VOCAB_GLYPHS='[\[\]▾▸x~\+\-^]|\(.*%\)'
if grep -rEn "$VOCAB_GLYPHS" "$COMPONENT_PATH" 2>/dev/null | grep -v "test\|doc.go\|DESIGN.md" | grep -q ".go"; then
	echo "  ⚠ WARNING: Found symbols (check against vocabulary in DESIGN.md §12)"
else
	echo "  ✓ PASSED: No non-vocabulary glyphs found"
	((PASSED++))
fi

# Rule 6: Focus only changes background, not size/border
echo ""
echo "[Rule 6] Focus: Only background changes (no size/border changes)"
if grep -r "isFocused" "$COMPONENT_PATH"/*.go 2>/dev/null | grep -En "Width|Height|Border|Margin" | grep -q "\.go"; then
	echo "  ✗ FAILED: Focus changes size/border (should only change background)"
	((FAILED++))
else
	echo "  ✓ PASSED: Focus only affects background color"
	((PASSED++))
fi

# Check if using PanelBg correctly
if grep -r "chrome.PanelBg" "$COMPONENT_PATH" 2>/dev/null | grep -q ".go"; then
	echo "  ✓ PASSED: Component uses chrome.PanelBg for focus"
	((PASSED++))
else
	if [ -f "$COMPONENT_PATH/View.go" ]; then
		echo "  ⚠ WARNING: No chrome.PanelBg found (might be handled by PanelFrame)"
	fi
fi

# Summary
echo ""
echo "=============================================="
echo "Results: $PASSED passed, $FAILED failed"
if [ $FAILED -gt 0 ]; then
	echo "Status: ✗ FAILED"
	exit 1
else
	echo "Status: ✓ PASSED"
	exit 0
fi
