package vault

type DatabaseCredentials struct {
	LeaseID string `json:"lease_id"`

	Renewable bool `json:"renewable"`

	LeaseDuration int64 `json:"lease_duration"`

	Data struct {
		Password string `json:"password"`
		Username string `json:"username"`
	} `json:"data"`
}
