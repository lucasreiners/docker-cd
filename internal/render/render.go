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

// StatusPage renders the full status page with ASCII art and container count.
func StatusPage(projectName string, runningContainers int) string {
	return fmt.Sprintf("%s\n  %s\n  Running containers: %d\n", asciiArt, projectName, runningContainers)
}
