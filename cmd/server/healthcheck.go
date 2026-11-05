package main

import (
	"net/http"
	"time"
)

// healthcheckURL is the address `serve` listens on inside the production
// container (see Dockerfile). PocketBase answers /api/health itself.
const healthcheckURL = "http://127.0.0.1:8090/api/health"

// runHealthcheck reports whether the server in this container answers, for
// Docker's HEALTHCHECK. The production image is distroless, so there is no
// curl or wget to do it.
//
// It is handled before pocketbase.New() rather than as a cobra subcommand on
// purpose: PocketBase bootstraps (opens pb_data) before running any
// subcommand, and a probe every ten seconds has no business opening the
// database the server is writing to.
func runHealthcheck() int {
	client := http.Client{Timeout: 3 * time.Second}

	res, err := client.Get(healthcheckURL)
	if err != nil {
		return 1
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return 1
	}

	return 0
}
