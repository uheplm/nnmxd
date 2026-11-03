package niri

import "testing"

func TestParseFocusedWindowGeometry(t *testing.T) {
	windowJSON := []byte(`{
		"app_id":"org.example.App",
		"layout":{"window_size":[960,720]}
	}`)
	outputJSON := []byte(`{"logical":{"width":1920,"height":1080,"scale":1.0}}`)

	got, err := parseFocusedWindowGeometry(windowJSON, outputJSON)
	if err != nil {
		t.Fatal(err)
	}
	if got.AppID != "org.example.App" ||
		got.WindowWidth != 960 || got.WindowHeight != 720 ||
		got.OutputWidth != 1920 || got.OutputHeight != 1080 {
		t.Fatalf("geometry = %#v", got)
	}
}

func TestParseFocusedWindowGeometryRejectsMissingDimensions(t *testing.T) {
	tests := map[string]struct {
		window string
		output string
	}{
		"missing window width": {
			window: `{"app_id":"org.example.App","layout":{"window_size":[]}}`,
			output: `{"logical":{"width":1920,"height":1080}}`,
		},
		"missing window height": {
			window: `{"app_id":"org.example.App","layout":{"window_size":[960]}}`,
			output: `{"logical":{"width":1920,"height":1080}}`,
		},
		"missing output width": {
			window: `{"app_id":"org.example.App","layout":{"window_size":[960,720]}}`,
			output: `{"logical":{"height":1080}}`,
		},
		"missing output height": {
			window: `{"app_id":"org.example.App","layout":{"window_size":[960,720]}}`,
			output: `{"logical":{"width":1920}}`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseFocusedWindowGeometry([]byte(test.window), []byte(test.output)); err == nil {
				t.Fatal("parseFocusedWindowGeometry succeeded without dimensions")
			}
		})
	}
}
