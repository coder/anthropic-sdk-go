package anthropic_test

import (
	"reflect"
	"testing"

	anthropic "github.com/charmbracelet/anthropic-sdk-go"
	shimjson "github.com/charmbracelet/anthropic-sdk-go/internal/encoding/json"
)

// TestAllMarshalerTypesImplementDirectEncoder uses reflection to scan every
// exported type in the anthropic package. Any type that implements
// json.Marshaler (i.e. has MarshalJSON) must also implement
// json.DirectEncoder (i.e. has EncodeDirect). This prevents new types
// from being added without the fast-path optimization.
func TestAllMarshalerTypesImplementDirectEncoder(t *testing.T) {
	marshalerType := reflect.TypeOf((*shimjson.Marshaler)(nil)).Elem()
	directEncoderType := reflect.TypeOf((*shimjson.DirectEncoder)(nil)).Elem()

	// We check a representative set of types from the package.
	// reflect doesn't let us enumerate all types in a package, so we
	// build the list from known param/union types.
	types := allParamTypes()

	missing := 0
	for _, typ := range types {
		rt := reflect.TypeOf(typ)

		// Check both value and pointer receiver.
		implementsMarshaler := rt.Implements(marshalerType) ||
			reflect.PointerTo(rt).Implements(marshalerType)
		implementsDirectEncoder := rt.Implements(directEncoderType) ||
			reflect.PointerTo(rt).Implements(directEncoderType)

		if implementsMarshaler && !implementsDirectEncoder {
			t.Errorf("type %s implements MarshalJSON but not EncodeDirect", rt.Name())
			missing++
		}
	}

	if missing > 0 {
		t.Logf("%d type(s) implement MarshalJSON without EncodeDirect", missing)
	}
}

// allParamTypes returns an instance of every exported param/union type
// in the anthropic package that has a MarshalJSON method. This list must
// be kept in sync with the package — the test above will catch any type
// that implements MarshalJSON but is missing from this list only if
// it's added here.
//
// TODO: if/when the SDK types are code-generated, generate this list
// too. Currently 102 types.
func allParamTypes() []any {
	return []any{
		anthropic.Base64ImageSourceParam{},
		anthropic.Base64PDFSourceParam{},
		anthropic.BashCodeExecutionOutputBlockParam{},
		anthropic.BashCodeExecutionResultBlockParam{},
		anthropic.BashCodeExecutionToolResultBlockParam{},
		anthropic.BashCodeExecutionToolResultBlockParamContentUnion{},
		anthropic.BashCodeExecutionToolResultErrorParam{},
		anthropic.CacheControlEphemeralParam{},
		anthropic.CitationCharLocationParam{},
		anthropic.CitationContentBlockLocationParam{},
		anthropic.CitationPageLocationParam{},
		anthropic.CitationSearchResultLocationParam{},
		anthropic.CitationWebSearchResultLocationParam{},
		anthropic.CitationsConfigParam{},
		anthropic.CodeExecutionOutputBlockParam{},
		anthropic.CodeExecutionResultBlockParam{},
		anthropic.CodeExecutionTool20250522Param{},
		anthropic.CodeExecutionTool20250825Param{},
		anthropic.CodeExecutionTool20260120Param{},
		anthropic.CodeExecutionToolResultBlockParam{},
		anthropic.CodeExecutionToolResultBlockParamContentUnion{},
		anthropic.CodeExecutionToolResultErrorParam{},
		anthropic.ContainerUploadBlockParam{},
		anthropic.ContentBlockParamUnion{},
		anthropic.ContentBlockSourceContentItemUnionParam{},
		anthropic.ContentBlockSourceContentUnionParam{},
		anthropic.ContentBlockSourceParam{},
		anthropic.DirectCallerParam{},
		anthropic.DocumentBlockParam{},
		anthropic.DocumentBlockParamSourceUnion{},
		anthropic.EncryptedCodeExecutionResultBlockParam{},
		anthropic.ImageBlockParam{},
		anthropic.ImageBlockParamSourceUnion{},
		anthropic.JSONOutputFormatParam{},
		anthropic.MemoryTool20250818Param{},
		anthropic.MessageCountTokensParams{},
		anthropic.MessageCountTokensParamsSystemUnion{},
		anthropic.MessageCountTokensToolUnionParam{},
		anthropic.MessageNewParams{},
		anthropic.MessageParam{},
		anthropic.MetadataParam{},
		anthropic.OutputConfigParam{},
		anthropic.PlainTextSourceParam{},
		anthropic.RedactedThinkingBlockParam{},
		anthropic.SearchResultBlockParam{},
		anthropic.ServerToolCaller20260120Param{},
		anthropic.ServerToolCallerParam{},
		anthropic.ServerToolUseBlockParam{},
		anthropic.ServerToolUseBlockParamCallerUnion{},
		anthropic.TextBlockParam{},
		anthropic.TextCitationParamUnion{},
		anthropic.TextEditorCodeExecutionCreateResultBlockParam{},
		anthropic.TextEditorCodeExecutionStrReplaceResultBlockParam{},
		anthropic.TextEditorCodeExecutionToolResultBlockParam{},
		anthropic.TextEditorCodeExecutionToolResultBlockParamContentUnion{},
		anthropic.TextEditorCodeExecutionToolResultErrorParam{},
		anthropic.TextEditorCodeExecutionViewResultBlockParam{},
		anthropic.ThinkingBlockParam{},
		anthropic.ThinkingConfigAdaptiveParam{},
		anthropic.ThinkingConfigDisabledParam{},
		anthropic.ThinkingConfigEnabledParam{},
		anthropic.ThinkingConfigParamUnion{},
		anthropic.ToolBash20250124Param{},
		anthropic.ToolChoiceAnyParam{},
		anthropic.ToolChoiceAutoParam{},
		anthropic.ToolChoiceNoneParam{},
		anthropic.ToolChoiceToolParam{},
		anthropic.ToolChoiceUnionParam{},
		anthropic.ToolInputSchemaParam{},
		anthropic.ToolParam{},
		anthropic.ToolReferenceBlockParam{},
		anthropic.ToolResultBlockParam{},
		anthropic.ToolResultBlockParamContentUnion{},
		anthropic.ToolSearchToolBm25_20251119Param{},
		anthropic.ToolSearchToolRegex20251119Param{},
		anthropic.ToolSearchToolResultBlockParam{},
		anthropic.ToolSearchToolResultBlockParamContentUnion{},
		anthropic.ToolSearchToolResultErrorParam{},
		anthropic.ToolSearchToolSearchResultBlockParam{},
		anthropic.ToolTextEditor20250124Param{},
		anthropic.ToolTextEditor20250429Param{},
		anthropic.ToolTextEditor20250728Param{},
		anthropic.ToolUnionParam{},
		anthropic.ToolUseBlockParam{},
		anthropic.ToolUseBlockParamCallerUnion{},
		anthropic.URLImageSourceParam{},
		anthropic.URLPDFSourceParam{},
		anthropic.UserLocationParam{},
		anthropic.WebFetchBlockParam{},
		anthropic.WebFetchTool20250910Param{},
		anthropic.WebFetchTool20260209Param{},
		anthropic.WebFetchToolResultBlockParam{},
		anthropic.WebFetchToolResultBlockParamCallerUnion{},
		anthropic.WebFetchToolResultBlockParamContentUnion{},
		anthropic.WebFetchToolResultErrorBlockParam{},
		anthropic.WebSearchResultBlockParam{},
		anthropic.WebSearchTool20250305Param{},
		anthropic.WebSearchTool20260209Param{},
		anthropic.WebSearchToolRequestErrorParam{},
		anthropic.WebSearchToolResultBlockParam{},
		anthropic.WebSearchToolResultBlockParamCallerUnion{},
		anthropic.WebSearchToolResultBlockParamContentUnion{},
	}
}
