package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/convox/cli/Godeps/_workspace/src/github.com/codegangsta/cli"
	"github.com/convox/cli/convox/build"
	"github.com/convox/cli/stdcli"
)

func init() {
	stdcli.RegisterCommand(cli.Command{
		Name:        "deploy",
		Description: "deploy an app to AWS",
		Usage:       "<directory>",
		Action:      cmdDeploy,
	})
}

func cmdDeploy(c *cli.Context) {
	base := "."

	if len(c.Args()) > 0 {
		base = c.Args()[0]
	}

	base, err := filepath.Abs(base)

	if err != nil {
		stdcli.Error(err)
		return
	}

	Build(base)

	m, err := build.ManifestFromPath(filepath.Join(base, "docker-compose.yml"))

	if err != nil {
		stdcli.Error(err)
		return
	}

	host, _, err := currentLogin()

	if err != nil {
		stdcli.Error(err)
		return
	}

	host = strings.Split(host, ":")[0] + ":5000"

	if os.Getenv("REGISTRY_HOST") != "" {
		host = os.Getenv("REGISTRY_HOST")
	}

	proj := strings.Replace(filepath.Base(base), "-", "", -1)
	tag := fmt.Sprintf("%v", stdcli.Tagger())
	tags := m.Tags(host, proj, tag)

	for tag, image := range tags {
		fmt.Printf("Tagging %s\n", image)
		err = stdcli.Run("docker", "tag", "-f", image, tag)

		if err != nil {
			stdcli.Error(err)
			return
		}

		fmt.Printf("Pushing %s\n", tag)
		err = stdcli.Run("docker", "push", tag)

		if err != nil {
			stdcli.Error(err)
			return
		}
	}

	// create app if it doesn't exist
	data, err := ConvoxGet(fmt.Sprintf("/apps/%s", proj))

	if err != nil {
		v := url.Values{}
		v.Set("name", proj)
		data, err = ConvoxPostForm("/apps", v)

		if err != nil {
			stdcli.Error(err)
			return
		}

		fmt.Printf("Created app %s\n", proj)

		// poll for complete
		for {
			data, err = ConvoxGet(fmt.Sprintf("/apps/%s/status", proj))

			if err != nil {
				stdcli.Error(err)
				return
			}

			if string(data) == "running" {
				fmt.Printf("Status %s\n", data)
				break
			}

			time.Sleep(1000 * time.Millisecond)
		}
	}

	// create release
	v := url.Values{}
	v.Set("manifest", m.String())
	v.Set("tag", tag)
	data, err = ConvoxPostForm(fmt.Sprintf("/apps/%s/releases", proj), v)

	if err != nil {
		stdcli.Error(err)
		return
	}

	fmt.Printf("Created release %s\n", tag)

	// poll for complete
	for {
		data, err = ConvoxGet(fmt.Sprintf("/apps/%s/status", proj))

		if err != nil {
			stdcli.Error(err)
			return
		}

		if string(data) == "running" {
			fmt.Printf("Status %s\n", data)
			break
		}

		time.Sleep(1000 * time.Millisecond)
	}
}