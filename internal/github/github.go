package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Project struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
}

type RepoInfo struct {
	Owner string
	Name  string
}

// IsAuthenticated checks if gh CLI is authenticated
func IsAuthenticated() bool {
	cmd := exec.Command("gh", "auth", "status")
	return cmd.Run() == nil
}

// HasRequiredScopes checks if the token has project and repo scopes
func HasRequiredScopes() (bool, error) {
	cmd := exec.Command("gh", "auth", "status", "-t")
	output, err := cmd.Output()
	if err != nil {
		return false, nil
	}

	// Parse output to check for scopes
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Token scopes:") {
			hasProject := strings.Contains(line, "'project'") || strings.Contains(line, "project")
			hasRepo := strings.Contains(line, "'repo'") || strings.Contains(line, "repo")
			return hasProject && hasRepo, nil
		}
	}

	return false, nil
}

// Authenticate triggers GitHub authentication with required scopes
func Authenticate() error {
	cmd := exec.Command("gh", "auth", "refresh", "-h", "github.com", "-s", "project", "-s", "repo")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// GetRepoInfo gets the current repository owner and name
func GetRepoInfo() (*RepoInfo, error) {
	cmd := exec.Command("gh", "repo", "view", "--json", "owner,name")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get repo info: %w", err)
	}

	var result struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse repo info: %w", err)
	}

	return &RepoInfo{
		Owner: result.Owner.Login,
		Name:  result.Name,
	}, nil
}

// ListProjects lists all GitHub Projects for a repository
func ListProjects(owner, repo string) ([]Project, error) {
	query := fmt.Sprintf(`
		query {
			repository(owner: "%s", name: "%s") {
				projectsV2(first: 10) {
					nodes {
						id
						number
						title
					}
				}
			}
		}
	`, owner, repo)

	output, err := runGraphQL(query)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			Repository struct {
				ProjectsV2 struct {
					Nodes []Project `json:"nodes"`
				} `json:"projectsV2"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse projects: %w", err)
	}

	return result.Data.Repository.ProjectsV2.Nodes, nil
}

// CreateProject creates a new GitHub Project
func CreateProject(owner, repo, title string) (*Project, error) {
	// Get both repository ID and owner ID
	repoQuery := fmt.Sprintf(`
		query {
			repository(owner: "%s", name: "%s") {
				id
				owner {
					id
				}
			}
		}
	`, owner, repo)

	output, err := runGraphQL(repoQuery)
	if err != nil {
		return nil, err
	}

	var repoResult struct {
		Data struct {
			Repository struct {
				ID    string `json:"id"`
				Owner struct {
					ID string `json:"id"`
				} `json:"owner"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(output, &repoResult); err != nil {
		return nil, fmt.Errorf("failed to parse repo ID: %w", err)
	}

	ownerID := repoResult.Data.Repository.Owner.ID
	repoID := repoResult.Data.Repository.ID

	// Create the project with the owner ID
	mutation := fmt.Sprintf(`
		mutation {
			createProjectV2(input: {
				ownerId: "%s"
				title: "%s"
			}) {
				projectV2 {
					id
					number
					title
				}
			}
		}
	`, ownerID, escapeString(title))

	output, err = runGraphQL(mutation)
	if err != nil {
		return nil, err
	}

	var createResult struct {
		Data struct {
			CreateProjectV2 struct {
				ProjectV2 Project `json:"projectV2"`
			} `json:"createProjectV2"`
		} `json:"data"`
	}

	if err := json.Unmarshal(output, &createResult); err != nil {
		return nil, fmt.Errorf("failed to parse project creation: %w", err)
	}

	project := createResult.Data.CreateProjectV2.ProjectV2

	// Link the project to the repository
	linkMutation := fmt.Sprintf(`
		mutation {
			linkProjectV2ToRepository(input: {
				projectId: "%s"
				repositoryId: "%s"
			}) {
				repository {
					id
				}
			}
		}
	`, project.ID, repoID)

	_, err = runGraphQL(linkMutation)
	if err != nil {
		// Don't fail if linking fails, project is still created
		fmt.Printf("Warning: failed to link project to repository: %v\n", err)
	}

	return &project, nil
}

func runGraphQL(query string) ([]byte, error) {
	cmd := exec.Command("gh", "api", "graphql", "-f", fmt.Sprintf("query=%s", query))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("GraphQL query failed: %s", stderr.String())
	}

	return output, nil
}

func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
