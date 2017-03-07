package database

import (
	"errors"
	"fmt"
	"time"

	"github.com/fatih/structs"
	"github.com/hashicorp/vault/builtin/logical/database/dbs"
	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

func pathResetConnection(b *databaseBackend) *framework.Path {
	return &framework.Path{
		Pattern: fmt.Sprintf("reset/%s", framework.GenericNameRegex("name")),
		Fields: map[string]*framework.FieldSchema{
			"name": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "Name of this DB type",
			},
		},

		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.UpdateOperation: b.pathConnectionReset,
		},

		HelpSynopsis:    pathConfigConnectionHelpSyn,
		HelpDescription: pathConfigConnectionHelpDesc,
	}
}

func (b *databaseBackend) pathConnectionReset(req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	name := data.Get("name").(string)
	if name == "" {
		return nil, errors.New("No database name set")
	}

	// Grab the mutex lock
	b.Lock()
	defer b.Unlock()

	entry, err := req.Storage.Get(fmt.Sprintf("dbs/%s", name))
	if err != nil {
		return nil, fmt.Errorf("failed to read connection configuration")
	}
	if entry == nil {
		return nil, nil
	}

	var config dbs.DatabaseConfig
	if err := entry.DecodeJSON(&config); err != nil {
		return nil, err
	}

	db, ok := b.connections[name]
	if !ok {
		return logical.ErrorResponse("Can not change type of existing connection."), nil
	}

	db.Close()
	db, err = dbs.Factory(&config)
	if err != nil {
		return logical.ErrorResponse(fmt.Sprintf("Error creating database object: %s", err)), nil
	}

	b.connections[name] = db

	return nil, nil
}

func pathConfigConnection(b *databaseBackend) *framework.Path {
	return &framework.Path{
		Pattern: fmt.Sprintf("dbs/%s", framework.GenericNameRegex("name")),
		Fields: map[string]*framework.FieldSchema{
			"name": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "Name of this DB type",
			},

			"connection_type": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "DB type (e.g. postgres)",
			},

			"verify_connection": &framework.FieldSchema{
				Type:        framework.TypeBool,
				Default:     true,
				Description: `If set, connection_url is verified by actually connecting to the database`,
			},

			"max_open_connections": &framework.FieldSchema{
				Type: framework.TypeInt,
				Description: `Maximum number of open connections to the database;
a zero uses the default value of two and a
negative value means unlimited`,
			},

			"max_idle_connections": &framework.FieldSchema{
				Type: framework.TypeInt,
				Description: `Maximum number of idle connections to the database;
a zero uses the value of max_open_connections
and a negative value disables idle connections.
If larger than max_open_connections it will be
reduced to the same size.`,
			},

			"max_connection_lifetime": &framework.FieldSchema{
				Type:    framework.TypeString,
				Default: "0s",
				Description: `Maximum amount of time a connection may be reused;
				a zero or negative value reuses connections forever.`,
			},
		},

		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.UpdateOperation: b.pathConnectionWrite,
			logical.ReadOperation:   b.pathConnectionRead,
		},

		HelpSynopsis:    pathConfigConnectionHelpSyn,
		HelpDescription: pathConfigConnectionHelpDesc,
	}
}

// pathConnectionRead reads out the connection configuration
func (b *databaseBackend) pathConnectionRead(req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	name := data.Get("name").(string)

	entry, err := req.Storage.Get(fmt.Sprintf("dbs/%s", name))
	if err != nil {
		return nil, fmt.Errorf("failed to read connection configuration")
	}
	if entry == nil {
		return nil, nil
	}

	var config dbs.DatabaseConfig
	if err := entry.DecodeJSON(&config); err != nil {
		return nil, err
	}
	return &logical.Response{
		Data: structs.New(config).Map(),
	}, nil
}

func (b *databaseBackend) pathConnectionWrite(req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	connType := data.Get("connection_type").(string)

	maxOpenConns := data.Get("max_open_connections").(int)
	if maxOpenConns == 0 {
		maxOpenConns = 2
	}

	maxIdleConns := data.Get("max_idle_connections").(int)
	if maxIdleConns == 0 {
		maxIdleConns = maxOpenConns
	}
	if maxIdleConns > maxOpenConns {
		maxIdleConns = maxOpenConns
	}

	maxConnLifetimeRaw := data.Get("max_connection_lifetime").(string)
	maxConnLifetime, err := time.ParseDuration(maxConnLifetimeRaw)
	if err != nil {
		return logical.ErrorResponse(fmt.Sprintf(
			"Invalid max_connection_lifetime: %s", err)), nil
	}

	config := &dbs.DatabaseConfig{
		DatabaseType:          connType,
		ConnectionDetails:     data.Raw,
		MaxOpenConnections:    maxOpenConns,
		MaxIdleConnections:    maxIdleConns,
		MaxConnectionLifetime: maxConnLifetime,
	}

	name := data.Get("name").(string)

	// Grab the mutex lock
	b.Lock()
	defer b.Unlock()

	var db dbs.DatabaseType
	if _, ok := b.connections[name]; ok {

		// Don't allow the connection type to change
		if b.connections[name].Type() != connType {
			return logical.ErrorResponse("Can not change type of existing connection."), nil
		}
	} else {
		db, err = dbs.Factory(config)
		if err != nil {
			return logical.ErrorResponse(fmt.Sprintf("Error creating database object: %s", err)), nil
		}

		b.connections[name] = db
	}

	/*
		// Don't check the connection_url if verification is disabled
		verifyConnection := data.Get("verify_connection").(bool)
		if verifyConnection {
			// Verify the string
			db, err := sql.Open("postgres", connURL)
			if err != nil {
				return logical.ErrorResponse(fmt.Sprintf(
					"Error validating connection info: %s", err)), nil
			}
			defer db.Close()
			if err := db.Ping(); err != nil {
				return logical.ErrorResponse(fmt.Sprintf(
					"Error validating connection info: %s", err)), nil
			}
		}
	*/

	// Store it
	entry, err := logical.StorageEntryJSON(fmt.Sprintf("dbs/%s", name), config)
	if err != nil {
		return nil, err
	}
	if err := req.Storage.Put(entry); err != nil {
		return nil, err
	}

	// Reset the DB connection
	resp := &logical.Response{}
	resp.AddWarning("Read access to this endpoint should be controlled via ACLs as it will return the connection string or URL as it is, including passwords, if any.")

	return resp, nil
}

const pathConfigConnectionHelpSyn = `
Configure the connection string to talk to PostgreSQL.
`

const pathConfigConnectionHelpDesc = `
This path configures the connection string used to connect to PostgreSQL.
The value of the string can be a URL, or a PG style string in the
format of "user=foo host=bar" etc.

The URL looks like:
"postgresql://user:pass@host:port/dbname"

When configuring the connection string, the backend will verify its validity.
`