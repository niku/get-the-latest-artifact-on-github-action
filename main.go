package main

import (
	"archive/zip"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"

	"github.com/google/go-github/v43/github"
	"golang.org/x/oauth2"
)

const (
	VERSION    = "0.0.1"
	REPOSITORY = "https://github.com/coop-sapporo/get-the-latest-artifact-on-github-action"
	// https://docs.github.com/en/rest/guides/traversing-with-pagination#basics-of-pagination
	MAX_NUMBER_PER_PAGE = 100
)

// assume embedded by ldflags
var (
	REVISION     string
	RELEASE_FLAG string
)

func main() {
	// Some cli tools(e.g. hub, gh) use GITHUB_TOKEN environment variable.
	// We provide that the token can be used as a straight forward way.
	githubToken := os.Getenv("GITHUB_TOKEN")
	var (
		owner string
		repo  string
	)
	flag.StringVar(&owner, "owner", "", "Repository owner")
	flag.StringVar(&repo, "repo", "", "Repository")
	flag.Parse()

	requiredParameters := []string{owner, repo}
	for _, v := range requiredParameters {
		if v == "" {
			fmt.Fprintln(os.Stderr, "Parameters owner, repo are required")
			flag.PrintDefaults()
			fmt.Fprintln(os.Stderr, "")
			printCodeInfo()
			os.Exit(1)
		}
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	githubClient := github.NewClient(tc)

	// list artifacts
	var artifacts []*github.Artifact
	page := 1
	for {
		// NOTE: At this moment, we don't care about huge number of pages. we assume a couple or few pages.
		artifactList, resp, err := githubClient.Actions.ListArtifacts(ctx, owner, repo, &github.ListOptions{PerPage: MAX_NUMBER_PER_PAGE, Page: page})
		if err != nil {
			log.Fatalf("unable to list artifacts. pqge: %d, detail: %+v", page, err)
		}
		artifacts = append(artifacts, artifactList.Artifacts...)
		page = resp.NextPage
		// if there are no additional pages
		if page == 0 {
			break
		}
	}

	// sort createdAt desc
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].GetCreatedAt().After(artifacts[j].GetCreatedAt().Time)
	})

	// get the newest artifact
	artifact := artifacts[0]
	artifactID := artifact.GetID()

	// make a download url
	url, _, err := githubClient.Actions.DownloadArtifact(ctx, owner, repo, artifactID, true)
	if err != nil {
		log.Fatalf("unable to get download url. detail: %+v", err)
	}

	// get an archive
	resp, err := http.Get(url.String())
	if err != nil {
		log.Fatalf("unable to get artifact. detail: %+v", err)
	}
	defer resp.Body.Close()

	temp, err := os.CreateTemp("", "tmpfile-latest-pdf-*.zip")
	if err != nil {
		log.Fatalf("unable to create temp file. detail: %+v", err)
	}
	defer func() {
		temp.Close()
		os.RemoveAll(temp.Name())
	}()

	if _, err := io.Copy(temp, resp.Body); err != nil {
		log.Fatalf("unable to copy response body to file. detail: %+v", err)
	}
	temp.Close()

	// unzip
	zipfile, err := zip.OpenReader(temp.Name())
	if err != nil {
		log.Fatalf("unable to open zip reader. detail: %+v", err)
	}
	defer zipfile.Close()
	for _, file := range zipfile.File {
		src, err := file.Open()
		if err != nil {
			log.Fatalf("unable to open src file. detail: %+v", err)
		}
		defer src.Close()

		dst, err := os.Create(file.Name)
		if err != nil {
			log.Fatalf("unable to create dst file. detail: %+v", err)
		}
		defer dst.Close()

		io.Copy(dst, src)
	}
}

func printCodeInfo() {
	var t []string

	t = append(t, "==== CODE INFOMATION ====")
	isRelease := RELEASE_FLAG != ""
	if isRelease {
		t = append(t, "VERSION: "+VERSION)
	} else {
		t = append(t, "VERSION: "+VERSION+"-dev")
	}
	t = append(t, "REVISION: "+REVISION)

	// These url structures are assumed below.
	// - The repository uses GitHub.
	// - Each tag name starts with 'v' then followed by version.
	t = append(t, fmt.Sprintf("URL(revision): %s/commit/%s", REPOSITORY, REVISION))
	if isRelease {
		t = append(t, fmt.Sprintf("URL(tag): %s/releases/tag/v%s", REPOSITORY, VERSION))
	}

	for _, line := range t {
		fmt.Fprintln(os.Stderr, line)
	}
}
