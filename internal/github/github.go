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

type ProjectItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Body    string `json:"body"`
	Content struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		URL    string `json:"url"`
	} `json:"content"`
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

// ListProjectItems fetches all items from a GitHub Project
func ListProjectItems(owner, repo string, projectNumber int) ([]ProjectItem, error) {
	// First, get the project ID
	projectQuery := fmt.Sprintf(`
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

	output, err := runGraphQL(projectQuery)
	if err != nil {
		return nil, err
	}

	var projectResult struct {
		Data struct {
			Repository struct {
				ProjectsV2 struct {
					Nodes []Project `json:"nodes"`
				} `json:"projectsV2"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(output, &projectResult); err != nil {
		return nil, fmt.Errorf("failed to parse projects: %w", err)
	}

	// Find the project with the matching number
	var projectID string
	for _, project := range projectResult.Data.Repository.ProjectsV2.Nodes {
		if project.Number == projectNumber {
			projectID = project.ID
			break
		}
	}

	if projectID == "" {
		return nil, fmt.Errorf("project #%d not found", projectNumber)
	}

	// Get the project items with status field
	itemsQuery := fmt.Sprintf(`
		query {
			node(id: "%s") {
				... on ProjectV2 {
					items(first: 100) {
						nodes {
							id
							fieldValues(first: 10) {
								nodes {
									... on ProjectV2ItemFieldSingleSelectValue {
										name
										field {
											... on ProjectV2SingleSelectField {
												name
											}
										}
									}
									... on ProjectV2ItemFieldTextValue {
										text
										field {
											... on ProjectV2FieldCommon {
												name
											}
										}
									}
								}
							}
							content {
								... on Issue {
									number
									title
									body
									url
								}
								... on DraftIssue {
									title
									body
								}
							}
						}
					}
				}
			}
		}
	`, projectID)

	output, err = runGraphQL(itemsQuery)
	if err != nil {
		return nil, err
	}

	var itemsResult struct {
		Data struct {
			Node struct {
				Items struct {
					Nodes []struct {
						ID          string `json:"id"`
						FieldValues struct {
							Nodes []struct {
								Name  string `json:"name"`
								Text  string `json:"text"`
								Field struct {
									Name string `json:"name"`
								} `json:"field"`
							} `json:"nodes"`
						} `json:"fieldValues"`
						Content struct {
							Number int    `json:"number"`
							Title  string `json:"title"`
							Body   string `json:"body"`
							URL    string `json:"url"`
						} `json:"content"`
					} `json:"nodes"`
				} `json:"items"`
			} `json:"node"`
		} `json:"data"`
	}

	if err := json.Unmarshal(output, &itemsResult); err != nil {
		return nil, fmt.Errorf("failed to parse project items: %w", err)
	}

	// Convert to ProjectItem
	var items []ProjectItem
	for _, node := range itemsResult.Data.Node.Items.Nodes {
		item := ProjectItem{
			ID:      node.ID,
			Title:   node.Content.Title,
			Content: node.Content,
		}

		// Extract status from field values
		for _, fv := range node.FieldValues.Nodes {
			if fv.Field.Name == "Status" {
				item.Status = fv.Name
				break
			}
		}

		items = append(items, item)
	}

	return items, nil
}

// CreateProjectItem creates a new item in a GitHub Project
func CreateProjectItem(owner, repo string, projectNumber int, title string) (*ProjectItem, error) {
	// First, get the project ID
	projectQuery := fmt.Sprintf(`
		query {
			repository(owner: "%s", name: "%s") {
				projectsV2(first: 10) {
					nodes {
						id
						number
					}
				}
			}
		}
	`, owner, repo)

	output, err := runGraphQL(projectQuery)
	if err != nil {
		return nil, err
	}

	var projectResult struct {
		Data struct {
			Repository struct {
				ProjectsV2 struct {
					Nodes []struct {
						ID     string `json:"id"`
						Number int    `json:"number"`
					} `json:"nodes"`
				} `json:"projectsV2"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(output, &projectResult); err != nil {
		return nil, fmt.Errorf("failed to parse projects: %w", err)
	}

	var projectID string
	for _, project := range projectResult.Data.Repository.ProjectsV2.Nodes {
		if project.Number == projectNumber {
			projectID = project.ID
			break
		}
	}

	if projectID == "" {
		return nil, fmt.Errorf("project #%d not found", projectNumber)
	}

	// Create a draft issue in the project
	mutation := fmt.Sprintf(`
		mutation {
			addProjectV2DraftIssue(input: {
				projectId: "%s"
				title: "%s"
			}) {
				projectItem {
					id
					content {
						... on DraftIssue {
							title
						}
					}
				}
			}
		}
	`, projectID, escapeString(title))

	output, err = runGraphQL(mutation)
	if err != nil {
		return nil, fmt.Errorf("failed to create project item: %w", err)
	}

	var createResult struct {
		Data struct {
			AddProjectV2DraftIssue struct {
				ProjectItem struct {
					ID      string `json:"id"`
					Content struct {
						Title string `json:"title"`
					} `json:"content"`
				} `json:"projectItem"`
			} `json:"addProjectV2DraftIssue"`
		} `json:"data"`
	}

	if err := json.Unmarshal(output, &createResult); err != nil {
		return nil, fmt.Errorf("failed to parse project item creation: %w", err)
	}

	return &ProjectItem{
		ID:    createResult.Data.AddProjectV2DraftIssue.ProjectItem.ID,
		Title: createResult.Data.AddProjectV2DraftIssue.ProjectItem.Content.Title,
	}, nil
}

// UpdateProjectItemStatus updates the status of a project item
func UpdateProjectItemStatus(owner, repo string, projectNumber int, itemID string, status string) error {
	// First, get the project ID and status field ID
	projectQuery := fmt.Sprintf(`
		query {
			repository(owner: "%s", name: "%s") {
				projectsV2(first: 10) {
					nodes {
						id
						number
						fields(first: 20) {
							nodes {
								... on ProjectV2SingleSelectField {
									id
									name
									options {
										id
										name
									}
								}
							}
						}
					}
				}
			}
		}
	`, owner, repo)

	output, err := runGraphQL(projectQuery)
	if err != nil {
		return err
	}

	var projectResult struct {
		Data struct {
			Repository struct {
				ProjectsV2 struct {
					Nodes []struct {
						ID     string `json:"id"`
						Number int    `json:"number"`
						Fields struct {
							Nodes []struct {
								ID      string `json:"id"`
								Name    string `json:"name"`
								Options []struct {
									ID   string `json:"id"`
									Name string `json:"name"`
								} `json:"options"`
							} `json:"nodes"`
						} `json:"fields"`
					} `json:"nodes"`
				} `json:"projectsV2"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(output, &projectResult); err != nil {
		return fmt.Errorf("failed to parse projects: %w", err)
	}

	// Find the project and status field
	var projectID, statusFieldID, statusOptionID string
	for _, project := range projectResult.Data.Repository.ProjectsV2.Nodes {
		if project.Number == projectNumber {
			projectID = project.ID
			// Find the Status field
			for _, field := range project.Fields.Nodes {
				if field.Name == "Status" {
					statusFieldID = field.ID
					// Find the option matching the desired status
					for _, option := range field.Options {
						if option.Name == status {
							statusOptionID = option.ID
							break
						}
					}
					break
				}
			}
			break
		}
	}

	if projectID == "" {
		return fmt.Errorf("project #%d not found", projectNumber)
	}
	if statusFieldID == "" {
		return fmt.Errorf("Status field not found in project")
	}
	if statusOptionID == "" {
		return fmt.Errorf("status option '%s' not found", status)
	}

	// Update the item status
	mutation := fmt.Sprintf(`
		mutation {
			updateProjectV2ItemFieldValue(input: {
				projectId: "%s"
				itemId: "%s"
				fieldId: "%s"
				value: {
					singleSelectOptionId: "%s"
				}
			}) {
				projectV2Item {
					id
				}
			}
		}
	`, projectID, itemID, statusFieldID, statusOptionID)

	_, err = runGraphQL(mutation)
	if err != nil {
		return fmt.Errorf("failed to update item status: %w", err)
	}

	return nil
}

func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// IssueComment represents a comment on a GitHub issue
type IssueComment struct {
	ID        int    `json:"id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
}

// GetIssueComments fetches all comments for a GitHub issue
func GetIssueComments(owner, repo string, issueNumber int) ([]IssueComment, error) {
	cmd := exec.Command("gh", "api",
		fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, issueNumber),
		"--jq", ".")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get issue comments: %w", err)
	}

	var comments []IssueComment
	if err := json.Unmarshal(output, &comments); err != nil {
		return nil, fmt.Errorf("failed to parse issue comments: %w", err)
	}

	return comments, nil
}

// CreateIssueComment creates a new comment on a GitHub issue
func CreateIssueComment(owner, repo string, issueNumber int, body string) error {
	// Create a JSON payload
	payload := map[string]string{
		"body": body,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal comment body: %w", err)
	}

	cmd := exec.Command("gh", "api",
		fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, issueNumber),
		"--method", "POST",
		"--input", "-")

	cmd.Stdin = bytes.NewReader(payloadBytes)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create issue comment: %s", stderr.String())
	}

	return nil
}
