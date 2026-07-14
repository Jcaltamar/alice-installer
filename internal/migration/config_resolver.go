package migration

import (
	"strconv"
	"strings"
)

const (
	developmentHost     = "127.0.0.1"
	developmentPort     = 5435
	developmentDatabase = "alice_guardian"
	developmentUsername = "postgres"
)

func resolveFields(environment EnvironmentName, fields configObject, host Environment) (ResolvedConfig, error) {
	for name := range fields {
		switch name {
		case "dialect", "database", "username", "password", "host", "port":
		default:
			return ResolvedConfig{}, configError()
		}
	}
	dialect, ok := resolveString(fields, "dialect", host)
	if !ok || dialect != DialectPostgreSQL {
		return ResolvedConfig{}, configError()
	}
	database, databaseOK := resolveString(fields, "database", host)
	username, usernameOK := resolveString(fields, "username", host)
	password, passwordOK := resolveString(fields, "password", host)
	address, hostOK := resolveString(fields, "host", host)
	port, portOK := resolvePort(fields, host)
	sources := map[string]ValueSource{
		"dialect": sourceFor(fields["dialect"], host), "database": sourceFor(fields["database"], host),
		"username": sourceFor(fields["username"], host), "password": sourceFor(fields["password"], host),
		"host": sourceFor(fields["host"], host), "port": sourceFor(fields["port"], host),
	}
	if environment == EnvironmentDevelopment {
		if !databaseOK {
			database, databaseOK, sources["database"] = developmentDatabase, true, ValueSourceDevelopmentDefault
		}
		if !usernameOK {
			username, usernameOK, sources["username"] = developmentUsername, true, ValueSourceDevelopmentDefault
		}
		if !hostOK {
			address, hostOK, sources["host"] = developmentHost, true, ValueSourceDevelopmentDefault
		}
		if !portOK {
			port, portOK, sources["port"] = developmentPort, true, ValueSourceDevelopmentDefault
		}
	} else if !portOK {
		port, portOK, sources["port"] = 5432, true, ValueSourcePostgreSQLDefault
	}
	if !databaseOK || !usernameOK || !passwordOK || !hostOK || !portOK || database == "" || username == "" || password == "" || address == "" || port < 1 || port > 65535 {
		return ResolvedConfig{}, configError()
	}
	return ResolvedConfig{Environment: environment, Dialect: dialect, Database: database, Username: username, Host: address, Port: port, Sources: sources, password: newSecret(password)}, nil
}
func sourceFor(value configValue, environment Environment) ValueSource {
	if value.kind == valueEnvironment {
		if override, found := environment.Lookup(value.env); found && strings.TrimSpace(override) != "" {
			return ValueSourceEnvironment
		}
	}
	return ValueSourceLiteral
}
func resolveString(fields configObject, name string, environment Environment) (string, bool) {
	value, ok := fields[name]
	if !ok {
		return "", false
	}
	value, ok = resolveValue(value, environment)
	if !ok || value.number != 0 {
		return "", false
	}
	return value.text, value.text != ""
}
func resolvePort(fields configObject, environment Environment) (int, bool) {
	value, exists := fields["port"]
	if !exists {
		return 0, false
	}
	value, ok := resolveValue(value, environment)
	if !ok {
		return 0, false
	}
	if value.number != 0 {
		return value.number, true
	}
	port, err := strconv.Atoi(value.text)
	if err != nil {
		return 0, false
	}
	return port, true
}
func resolveValue(value configValue, environment Environment) (configValue, bool) {
	if value.kind == valueLiteral {
		return value, true
	}
	if value.kind != valueEnvironment || value.fallback == nil {
		return configValue{}, false
	}
	if override, found := environment.Lookup(value.env); found && strings.TrimSpace(override) != "" {
		return configValue{kind: valueLiteral, text: override}, true
	}
	return *value.fallback, true
}
