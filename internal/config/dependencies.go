package config

// RequiresMySQL describes the adapters actually wired by cmd/api. Optional
// connection settings alone must not make the in-memory demo depend on a DB.
func (c Config) RequiresMySQL() bool {
	return c.CampaignRepository == "mysql" || c.DecisionStore == "mysql" ||
		c.ProfileStore == "mysql" || c.ProfileStore == "mysql-redis" ||
		c.AuthStore == "mysql" || c.AuditStore == "mysql" || c.EventTransport == "kafka"
}

func (c Config) RequiresRedis() bool {
	return c.ReservationAdapter == "redis" || c.ProfileStore == "mysql-redis" ||
		c.DecisionRateLimiter == "redis" || c.EventTransport == "kafka"
}
