package jsonvalue

import "testing"

func TestEqual(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		left, right string
		want        bool
		wantErr     bool
	}{
		"identical":                    {left: `{"a":1}`, right: `{"a":1}`, want: true},
		"whitespace":                   {left: `{"a":1}`, right: "{\n  \"a\": 1\n}", want: true},
		"key order":                    {left: `{"a":1,"b":2}`, right: `{"b":2,"a":1}`, want: true},
		"different value":              {left: `{"a":1}`, right: `{"a":2}`},
		"array order matters":          {left: `[1,2]`, right: `[2,1]`},
		"large numbers aren't rounded": {left: `12345678901234567890`, right: `12345678901234567891`},
		"number spelling matters":      {left: `{"a":1}`, right: `{"a":1.0}`},
		"invalid left":                 {left: `{"a":`, right: `{"a":1}`, wantErr: true},
		"invalid right":                {left: `{"a":1}`, right: ``, wantErr: true},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := Equal(tc.left, tc.right)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Equal() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("Equal() = %v, want %v", got, tc.want)
			}
		})
	}
}
