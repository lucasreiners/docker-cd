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

func TestExtractComposeImages(t *testing.T) {
	cases := []struct {
		name string
		content string
		want []string
	}{
		{
			"singleImage",
			"services:\n  web:\n    image: nginx:alpine\n",
			[]string{"nginx:alpine"},
		},
		{
			"multipleServices",
			"services:\n  web:\n    image: nginx:alpine\n  db:\n    image: postgres:16\n",
			[]string{"nginx:alpine", "postgres:16"},
		},
		{
			"buildOnly",
			"services:\n  app:\n    build: .\n",
			nil,
		},
		{
			"buildAndImage",
			"services:\n  app:\n    build: .\n    image: myapp:latest\n",
			[]string{"myapp:latest"},
		},
		{
			"deduplication",
			"services:\n  web1:\n    image: nginx:alpine\n  web2:\n    image: nginx:alpine\n",
			[]string{"nginx:alpine"},
		},
		{
			"emptyContent",
			"",
			nil,
		},
		{
			"quotedValue",
			"services:\n  web:\n    image: \"nginx:alpine\"\n",
			[]string{"nginx:alpine"},
		},
		{
			"singleQuotedValue",
			"services:\n  web:\n    image: 'nginx:alpine'\n",
			[]string{"nginx:alpine"},
		},
		{
			"withVersionAndVolumes",
			"version: '3.8'\nservices:\n  web:\n    image: nginx:alpine\n  db:\n    image: postgres:16\nvolumes:\n  data:\n",
			[]string{"nginx:alpine", "postgres:16"},
		},
		{
			"4spaceIndent",
			"services:\n    web:\n        image: nginx:alpine\n",
			[]string{"nginx:alpine"},
		},
		{
			"mixedBuildAndImage",
			"services:\n  frontend:\n    build: ./frontend\n  backend:\n    image: node:20\n  db:\n    image: postgres:16\n",
			[]string{"node:20", "postgres:16"},
		},
		{
			"commentedImageLine",
			"services:\n  web:\n    # image: old:tag\n    image: nginx:alpine\n",
			[]string{"nginx:alpine"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractComposeImages([]byte(tc.content))
			if len(got) != len(tc.want) {
				t.Fatalf("got %v (len %d), want %v (len %d)", got, len(got), tc.want, len(tc.want))
			}
			for i, img := range got {
				if img != tc.want[i] {
					t.Errorf("image[%d]: got %q, want %q", i, img, tc.want[i])
				}
			}
		})
	}
}
