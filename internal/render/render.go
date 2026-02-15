package render

import "fmt"

// asciiArt is the fixed ASCII art banner for Docker-CD.
const asciiArt = `
 ____             _                ____ ____
|  _ \  ___   ___| | _____ _ __  / ___/ _  \
| | | |/ _ \ / __| |/ / _ \ '__|| |   | | | |
| |_| | (_) | (__|   <  __/ |   | |___| |_| |
|____/ \___/ \___|_|\_\___|_|    \____|____/
`

// RepoInfo holds non-secret repository configuration for display.
type RepoInfo struct {
	URL       string
	Revision  string
	DeployDir string
}

// StatusPage renders the full status page with ASCII art, container count,
// and optional repository information.
func StatusPage(projectName string, runningContainers int, repo *RepoInfo) string {
	page := fmt.Sprintf("%s\n  %s\n  Running containers: %d\n", asciiArt, projectName, runningContainers)

	if repo != nil {
		page += fmt.Sprintf("  Repository: %s\n", repo.URL)
		page += fmt.Sprintf("  Revision: %s\n", repo.Revision)
		dir := repo.DeployDir
		if dir == "" {
			dir = "/"
		}
		page += fmt.Sprintf("  Deploy dir: %s\n", dir)
	}

	return page
}
