package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/cavaliercoder/grab"
	"github.com/mholt/archiver"
	"golang.org/x/oauth2"
	"log"
	"os"
	"os/exec"
	"time"
)
import "github.com/google/go-github/github" // with go modules disabled

func main() {
	token := flag.String("token", "", "github access token")
	account := flag.String("account", "", "org to backup")
	migrationId := flag.Int64("migrationId", 0, "migrationId if you want to resume.")
	gitServer := flag.String("gitServer", "", "gitServer to push code to.")
	flag.Parse()

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *token},
	)

	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	// list all repositories for the authenticated user
	orgs, _, err := client.Repositories.ListByOrg(ctx, *account, &github.RepositoryListByOrgOptions{
		Type: "private",
	})
	if err != nil {
		panic(err)
	}
	var migration *github.Migration

	var repos []string
	for _, org := range orgs {
		fmt.Printf("%s\n", *org.FullName)
		repos = append(repos, *org.FullName)
	}

	if *migrationId != 0 {
		migration, _, err = client.Migrations.MigrationStatus(ctx, *account, *migrationId)
		if err != nil {
			panic(err)
		}

	} else {

		migration, _, err = client.Migrations.StartMigration(ctx, *account, repos, &github.MigrationOptions{
			LockRepositories:   false,
			ExcludeAttachments: false,
		})
		if err != nil {
			panic(err)
		}
		defer func() {
			for _, repo := range repos {
				_, err = client.Migrations.UnlockRepo(ctx, *account, *migration.ID, repo)
				if err != nil {
					panic(err)
				}
			}
		}()
	}

	zipPath := "./" + *account + ".tar.gz"
	_, err = os.Stat(zipPath)
	if os.IsNotExist(err) {

		for {
			migration, _, err := client.Migrations.MigrationStatus(ctx, *account, *migration.ID)
			if err != nil {
				panic(err)
			}

			fmt.Printf("migration_id: %d: %s\n", *migration.ID, *migration.State)

			if *migration.State == "exported" {
				break
			}

			if *migration.State == "failed" {
				panic("migration.State == failed")
			}

			time.Sleep(10 * time.Second)
		}

		url, err := client.Migrations.MigrationArchiveURL(ctx, *account, *migration.ID)
		if err != nil {
			panic(err)
		}

		resp, err := grab.Get(zipPath, url)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Download saved to", resp.Filename)
	}

	_, err = os.Stat(*account)
	if os.IsNotExist(err) {
		err = archiver.Unarchive(zipPath, *account)
	}

	_, err = os.Stat("repositories")
	if os.IsNotExist(err) {
		for _, repo := range repos {
			repoPath := fmt.Sprintf("%s/repositories/%s.git", *account, repo)
			info, err := os.Stat(repoPath)
			if !os.IsNotExist(err) && info.IsDir() {
				execSh(".", exec.Command("git", "clone", repoPath, fmt.Sprintf("./repositories/%s", repo)))
				if *gitServer != "" {
					execSh(fmt.Sprintf("./repositories/%s", repo), exec.Command("git", "remote", "set-url", "origin", fmt.Sprintf("%s/%s.git", *gitServer,repo)))
					execSh(fmt.Sprintf("./repositories/%s", repo), exec.Command("git", "push"))
				}
			}
		}
	}
}
func execSh(dir string, cmd *exec.Cmd) {
	cmd.Stdout = os.Stdout
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
	}
}
