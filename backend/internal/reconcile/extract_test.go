package reconcile

import "testing"

func TestExtractServiceNames_EdgeCases(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"standard", "services:\n  web:\n    image: nginx:alpine\n", 1},
		{"tabIndent", "services:\n\tweb:\n\t\timage: nginx:alpine\n", 1},
		{"4spaces", "services:\n    web:\n        image: nginx:alpine\n", 1},
		{"version", "version: '3.8'\nservices:\n  web:\n    image: nginx:alpine\n", 1},
		{"emptyLines", "\n\nservices:\n  web:\n    image: nginx:alpine\n\n", 1},
		{"withVolumes", "services:\n  web:\n    image: nginx:alpine\nvolumes:\n  data:\n", 1},
		{"multiSvc", "services:\n  web:\n    image: nginx\n  api:\n    image: node\n", 2},
		{"comment", "# my compose\nservices:\n  web:\n    image: nginx\n", 1},
		{"trailingSpace", "services: \n  web:\n    image: nginx\n", 1},
		{"noTrailingNL", "services:\n  web:\n    image: nginx:alpine", 1},
		{"windowsCRLF", "services:\r\n  web:\r\n    image: nginx:alpine\r\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractServiceNames([]byte(tc.content))
			if len(got) != tc.want {
				t.Errorf("got %v (len %d), want len %d", got, len(got), tc.want)
			}
		})
	}
}
