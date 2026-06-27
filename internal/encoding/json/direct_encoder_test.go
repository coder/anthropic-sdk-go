package json

import (
	"testing"
)

// --- Test 1: DirectEncoder is preferred over MarshalJSON ---

type directEncoderTest struct {
	From string
}

func (d directEncoderTest) MarshalJSON() ([]byte, error) {
	return []byte(`{"from":"marshal_json"}`), nil
}

func (d directEncoderTest) EncodeDirect() (any, bool) {
	return map[string]string{"from": "direct"}, true
}

// Compile-time interface check.
var _ DirectEncoder = directEncoderTest{}

func TestDirectEncoder_UsedBeforeMarshalJSON(t *testing.T) {
	v := directEncoderTest{From: "original"}
	b, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got := string(b)
	want := `{"from":"direct"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// --- Test 2: Fallback to MarshalJSON when EncodeDirect returns false ---

type directEncoderFallbackTest struct {
	From string
}

func (d directEncoderFallbackTest) MarshalJSON() ([]byte, error) {
	return []byte(`{"from":"marshal_json"}`), nil
}

func (d directEncoderFallbackTest) EncodeDirect() (any, bool) {
	return nil, false
}

var _ DirectEncoder = directEncoderFallbackTest{}

func TestDirectEncoder_FallbackToMarshalJSON(t *testing.T) {
	v := directEncoderFallbackTest{From: "original"}
	b, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got := string(b)
	want := `{"from":"marshal_json"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// --- Test 3: EncodeDirect returns nil, true → produces "null" ---

type directEncoderNilTest struct{}

func (d directEncoderNilTest) MarshalJSON() ([]byte, error) {
	return []byte(`{"from":"marshal_json"}`), nil
}

func (d directEncoderNilTest) EncodeDirect() (any, bool) {
	return nil, true
}

var _ DirectEncoder = directEncoderNilTest{}

func TestDirectEncoder_NilUnderlyingReturnsNull(t *testing.T) {
	v := directEncoderNilTest{}
	b, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got := string(b)
	want := `null`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// --- Test 4: Pointer receiver → goes through addrMarshalerEncoder ---

type directEncoderPtrReceiver struct {
	From string
}

func (d *directEncoderPtrReceiver) MarshalJSON() ([]byte, error) {
	return []byte(`{"from":"marshal_json"}`), nil
}

func (d *directEncoderPtrReceiver) EncodeDirect() (any, bool) {
	return map[string]string{"from": "direct_ptr"}, true
}

var _ DirectEncoder = (*directEncoderPtrReceiver)(nil)

func TestDirectEncoder_PointerReceiver(t *testing.T) {
	// Embedding in a struct field so the encoder takes the address
	// and goes through the addrMarshalerEncoder / condAddrEncoder path.
	// Marshal(&v) is required so struct fields are addressable.
	type wrapper struct {
		Inner directEncoderPtrReceiver `json:"inner"`
	}
	v := wrapper{Inner: directEncoderPtrReceiver{From: "original"}}
	b, err := Marshal(&v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got := string(b)
	want := `{"inner":{"from":"direct_ptr"}}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// --- Test 5: EncodeDirect returns a struct with json tags ---

type directEncoderTagsTest struct{}

func (d directEncoderTagsTest) MarshalJSON() ([]byte, error) {
	return []byte(`{"from":"marshal_json"}`), nil
}

func (d directEncoderTagsTest) EncodeDirect() (any, bool) {
	type tagged struct {
		MyField string `json:"my_field"`
		Empty   string `json:"empty,omitempty"`
	}
	return tagged{MyField: "hello", Empty: ""}, true
}

var _ DirectEncoder = directEncoderTagsTest{}

func TestDirectEncoder_PreservesStructTags(t *testing.T) {
	v := directEncoderTagsTest{}
	b, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got := string(b)
	want := `{"my_field":"hello"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// --- Test 6: Nested DirectEncoders ---

type innerDirectEncoder struct {
	Value string
}

func (d innerDirectEncoder) MarshalJSON() ([]byte, error) {
	return []byte(`{"from":"inner_marshal_json"}`), nil
}

func (d innerDirectEncoder) EncodeDirect() (any, bool) {
	return map[string]string{"value": d.Value}, true
}

var _ DirectEncoder = innerDirectEncoder{}

type outerDirectEncoder struct {
	Inner innerDirectEncoder
}

func (d outerDirectEncoder) MarshalJSON() ([]byte, error) {
	return []byte(`{"from":"outer_marshal_json"}`), nil
}

func (d outerDirectEncoder) EncodeDirect() (any, bool) {
	type shadow struct {
		Inner innerDirectEncoder `json:"inner"`
	}
	return shadow{Inner: d.Inner}, true
}

var _ DirectEncoder = outerDirectEncoder{}

func TestDirectEncoder_NestedDirectEncoders(t *testing.T) {
	v := outerDirectEncoder{
		Inner: innerDirectEncoder{Value: "nested_val"},
	}
	b, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got := string(b)
	want := `{"inner":{"value":"nested_val"}}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// --- Test 7: DirectEncoder on a union-like type ---
// Simulates the SDK union pattern where exactly one pointer field is non-nil.

type directEncoderUnionInner struct {
	Value string `json:"value"`
}

func (d directEncoderUnionInner) MarshalJSON() ([]byte, error) {
	return Marshal(struct {
		Value string `json:"value"`
	}{d.Value})
}

type directEncoderUnion struct {
	OfText  *directEncoderUnionInner
	OfImage *directEncoderUnionInner
}

func (u directEncoderUnion) MarshalJSON() ([]byte, error) {
	// Would normally call param.MarshalUnion
	if u.OfText != nil {
		return Marshal(u.OfText)
	}
	if u.OfImage != nil {
		return Marshal(u.OfImage)
	}
	return []byte("null"), nil
}

func (u directEncoderUnion) EncodeDirect() (any, bool) {
	if u.OfText != nil {
		return u.OfText, true
	}
	if u.OfImage != nil {
		return u.OfImage, true
	}
	return nil, true // null
}

func TestDirectEncoder_UnionType(t *testing.T) {
	// Test with OfText set
	u := directEncoderUnion{OfText: &directEncoderUnionInner{Value: "hello"}}
	b, err := Marshal(u)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got := string(b)
	want := `{"value":"hello"}`
	if got != want {
		t.Errorf("OfText: got %s, want %s", got, want)
	}

	// Test with OfImage set
	u2 := directEncoderUnion{OfImage: &directEncoderUnionInner{Value: "img"}}
	b2, err := Marshal(u2)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got2 := string(b2)
	want2 := `{"value":"img"}`
	if got2 != want2 {
		t.Errorf("OfImage: got %s, want %s", got2, want2)
	}

	// Test with nothing set (null)
	u3 := directEncoderUnion{}
	b3, err := Marshal(u3)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got3 := string(b3)
	if got3 != "null" {
		t.Errorf("empty union: got %s, want null", got3)
	}
}
