package jsonvalue

import "testing"

func FuzzDecodeFailsClosed(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`null`),
		[]byte(`{"value":[1,true,"text"]}`),
		[]byte(`{"duplicate":1,"duplicate":2}`),
		[]byte(`[[[[0]]]]`),
		[]byte{'"', 0xff, '"'},
	} {
		f.Add(seed)
	}

	limits := Limits{
		MaxBytes:       8 << 10,
		MaxDepth:       32,
		MaxNodes:       512,
		MaxNumberBytes: 128,
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		first, firstErr := Decode(data, limits)
		second, secondErr := Decode(data, limits)
		if (firstErr == nil) != (secondErr == nil) {
			t.Fatalf("Decode is nondeterministic: first=%#v/%v second=%#v/%v", first, firstErr, second, secondErr)
		}
		if firstErr == nil {
			if err := Validate(data, limits); err != nil {
				t.Fatalf("Validate rejected data accepted by Decode: %v", err)
			}
		}
	})
}
