package dto

type TestGitLabConnectionRequest struct {
	InstanceURL string `json:"instance_url" binding:"required,url"`
	Token       string `json:"token" binding:"required,min=10"`
}

type TestGitLabConnectionResponse struct {
	Username     string `json:"username"`
	ProjectCount int    `json:"project_count"`
}
