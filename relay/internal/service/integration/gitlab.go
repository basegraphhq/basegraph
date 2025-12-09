package integration

/*
	GitLab integration service
	Takes a PAT and url
	saves it as an integration, adds this token as integration_credential

	for each selected repositories,
		we create entries in repository table
		install webhooks through gitlab's api

*/
