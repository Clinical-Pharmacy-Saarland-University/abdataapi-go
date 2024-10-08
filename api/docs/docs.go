// Package docs Code generated by swaggo/swag. DO NOT EDIT
package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/admin/users": {
            "get": {
                "security": [
                    {
                        "Bearer": []
                    }
                ],
                "description": "__Admin role required__\nGet a list of users and their information based on optional query filters.\nSoft-deleted users are not included in the response.",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Admin"
                ],
                "summary": "Get user profile table",
                "parameters": [
                    {
                        "enum": [
                            "admin",
                            "user",
                            "approver"
                        ],
                        "type": "string",
                        "description": "Filter by role",
                        "name": "role",
                        "in": "query"
                    },
                    {
                        "enum": [
                            "active",
                            "inactive"
                        ],
                        "type": "string",
                        "description": "Filter by status",
                        "name": "status",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Admin table",
                        "schema": {
                            "$ref": "#/definitions/JSendSuccess-array_model_User"
                        }
                    },
                    "400": {
                        "description": "Bad request",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "403": {
                        "description": "Non-admin user",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "404": {
                        "description": "No users found",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "422": {
                        "description": "Bad query format",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ValidationResponse"
                        }
                    },
                    "500": {
                        "description": "Internal server error",
                        "schema": {
                            "$ref": "#/definitions/JSendError"
                        }
                    }
                }
            },
            "post": {
                "security": [
                    {
                        "Bearer": []
                    }
                ],
                "description": "__Admin role required__\nCreate a new user for the API. Ths user will receive an email with a token to set their password.\nYou can create users with the following roles: ` + "`" + `admin` + "`" + `, ` + "`" + `user` + "`" + `, ` + "`" + `approver` + "`" + `.",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Admin"
                ],
                "summary": "Create a new user",
                "parameters": [
                    {
                        "description": "Request body",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/CreateUserQuery"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "User created",
                        "schema": {
                            "$ref": "#/definitions/JSendSuccess-map_string_string"
                        }
                    },
                    "400": {
                        "description": "Bad request",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "403": {
                        "description": "Non-admin user",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "422": {
                        "description": "Bad query format",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ValidationResponse"
                        }
                    },
                    "500": {
                        "description": "Internal server error",
                        "schema": {
                            "$ref": "#/definitions/JSendError"
                        }
                    }
                }
            }
        },
        "/admin/users/{email}": {
            "get": {
                "security": [
                    {
                        "Bearer": []
                    }
                ],
                "description": "__Admin role required__\nGet the profile of a user based on the email address.\nSoft-deleted users can not be retrieved.",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Admin"
                ],
                "summary": "Get profile of a user",
                "parameters": [
                    {
                        "type": "string",
                        "description": "User email",
                        "name": "email",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "User profile",
                        "schema": {
                            "$ref": "#/definitions/JSendSuccess-model_User"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "403": {
                        "description": "Non-admin user",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "404": {
                        "description": "No users found",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal server error",
                        "schema": {
                            "$ref": "#/definitions/JSendError"
                        }
                    }
                }
            },
            "delete": {
                "security": [
                    {
                        "Bearer": []
                    }
                ],
                "description": "__Admin role required__\nDelete a user based on the email address.\nOnly soft-deletes the user, does not remove the user from the database.\nAdmins cannot delete their own account.",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Admin"
                ],
                "summary": "Delete a user",
                "parameters": [
                    {
                        "type": "string",
                        "description": "User email to delete",
                        "name": "email",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "User soft-deleted",
                        "schema": {
                            "$ref": "#/definitions/JSendSuccess-map_string_string"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "403": {
                        "description": "Non-admin user or cannot delete own account",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "404": {
                        "description": "User not found",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal server error",
                        "schema": {
                            "$ref": "#/definitions/JSendError"
                        }
                    }
                }
            },
            "patch": {
                "security": [
                    {
                        "Bearer": []
                    }
                ],
                "description": "__Admin role required__\nChange the role or status of a user based on the email address.\nAdmins cannot change their own role or status.\nPossible roles: ` + "`" + `admin` + "`" + `, ` + "`" + `user` + "`" + `, ` + "`" + `approver` + "`" + `.\nPossible statuses: ` + "`" + `active` + "`" + `, ` + "`" + `inactive` + "`" + `.",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Admin"
                ],
                "summary": "Change user role or status",
                "parameters": [
                    {
                        "type": "string",
                        "description": "User email to update",
                        "name": "email",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "Request body",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/ChangeUserProfileQuery"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "User profile updated",
                        "schema": {
                            "$ref": "#/definitions/JSendSuccess-map_string_string"
                        }
                    },
                    "400": {
                        "description": "No changes requested",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "403": {
                        "description": "Non-admin user or cannot update own account",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "404": {
                        "description": "User not found",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ErrorResponse"
                        }
                    },
                    "422": {
                        "description": "Bad query format",
                        "schema": {
                            "$ref": "#/definitions/JSendFailure-ValidationResponse"
                        }
                    },
                    "500": {
                        "description": "Internal server error",
                        "schema": {
                            "$ref": "#/definitions/JSendError"
                        }
                    }
                }
            }
        },
        "/adr": {
            "get": {
                "description": "Get ADRs for one or more PZNs. Each PZN can have multiple ADRs.\nThe ` + "`" + `lang` + "`" + ` parameter can be used to specify the language of the ADR descriptions.\nValid values are ` + "`" + `english` + "`" + `, ` + "`" + `german` + "`" + `, and ` + "`" + `german-simple` + "`" + `.\nThe default language is ` + "`" + `english` + "`" + `.\n` + "`" + `german-simple` + "`" + ` returns the simplified German ADR description.",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Adverse Drug Reactions"
                ],
                "summary": "List ADRs for PZNs",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Comma-separated list of PZNs",
                        "name": "pzns",
                        "in": "query",
                        "required": true
                    },
                    {
                        "enum": [
                            "english",
                            "german",
                            "german-simple"
                        ],
                        "type": "string",
                        "description": "Language for ADR names (default: english)",
                        "name": "lang",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "List of PZNs with ADRs",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/adrcontroller.PznADR"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad request (e.g. invalid PZNs)"
                    },
                    "404": {
                        "description": "PZN(s) not found"
                    }
                }
            }
        },
        "/formulations": {
            "get": {
                "security": [
                    {
                        "Bearer": []
                    }
                ],
                "description": "Drug formulation codes and their descriptions that are used in the database.\nThese codes are used, e.g., in the compound interaction endpoint.",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Formulation"
                ],
                "summary": "List all drug formulation codes and their descriptions",
                "responses": {
                    "200": {
                        "description": "Response with formulations",
                        "schema": {
                            "$ref": "#/definitions/FormResponse"
                        }
                    }
                }
            }
        },
        "/interactions/compounds": {
            "get": {
                "responses": {}
            }
        },
        "/sys/info": {
            "get": {
                "description": "Get information about the API including version and query limits.",
                "produces": [
                    "application/json",
                    "application/json"
                ],
                "tags": [
                    "System"
                ],
                "summary": "Get API Info",
                "responses": {
                    "200": {
                        "description": "Response with API info",
                        "schema": {
                            "$ref": "#/definitions/InfoResp"
                        }
                    }
                }
            }
        },
        "/sys/ping": {
            "get": {
                "description": "Ping the API to check if it is alive.",
                "produces": [
                    "application/json",
                    "application/json"
                ],
                "tags": [
                    "System"
                ],
                "summary": "Ping the API",
                "responses": {
                    "200": {
                        "description": "Response with pong message",
                        "schema": {
                            "$ref": "#/definitions/PingResp"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "ChangeUserProfileQuery": {
            "type": "object",
            "properties": {
                "role": {
                    "type": "string",
                    "enum": [
                        "admin",
                        "user",
                        "approver"
                    ],
                    "example": "user"
                },
                "status": {
                    "type": "string",
                    "enum": [
                        "active",
                        "inactive"
                    ],
                    "example": "inactive"
                }
            }
        },
        "CreateUserQuery": {
            "type": "object",
            "required": [
                "email",
                "first_name",
                "last_name",
                "organization",
                "role"
            ],
            "properties": {
                "email": {
                    "type": "string",
                    "maxLength": 255,
                    "minLength": 2,
                    "example": "joe@gmail.com"
                },
                "first_name": {
                    "type": "string",
                    "maxLength": 255,
                    "minLength": 2,
                    "example": "Joe"
                },
                "last_name": {
                    "type": "string",
                    "maxLength": 255,
                    "minLength": 2,
                    "example": "Doe"
                },
                "organization": {
                    "type": "string",
                    "maxLength": 255,
                    "minLength": 2,
                    "example": "ACME"
                },
                "role": {
                    "type": "string",
                    "enum": [
                        "admin",
                        "user",
                        "approver"
                    ]
                }
            }
        },
        "ErrorResponse": {
            "type": "object",
            "properties": {
                "error": {
                    "description": "Error message",
                    "type": "string",
                    "example": "Some error message"
                }
            }
        },
        "FormResponse": {
            "type": "object",
            "properties": {
                "formulations": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/Formulation"
                    }
                }
            }
        },
        "Formulation": {
            "type": "object",
            "properties": {
                "description": {
                    "description": "Formulation description",
                    "type": "string",
                    "example": "Tablet"
                },
                "formulation": {
                    "description": "Formulation code",
                    "type": "string",
                    "example": "TAB"
                }
            }
        },
        "InfoResp": {
            "type": "object",
            "properties": {
                "api_limits": {
                    "description": "Limits",
                    "allOf": [
                        {
                            "$ref": "#/definitions/cfg.LimitsConfig"
                        }
                    ]
                },
                "meta_info": {
                    "description": "Meta",
                    "allOf": [
                        {
                            "$ref": "#/definitions/cfg.MetaConfig"
                        }
                    ]
                }
            }
        },
        "JSendError": {
            "type": "object",
            "properties": {
                "message": {
                    "description": "Error message",
                    "type": "string",
                    "example": "Internal server error"
                },
                "status": {
                    "description": "Status",
                    "type": "string",
                    "example": "error"
                }
            }
        },
        "JSendFailure-ErrorResponse": {
            "type": "object",
            "properties": {
                "data": {
                    "description": "Data with error message(s)",
                    "allOf": [
                        {
                            "$ref": "#/definitions/ErrorResponse"
                        }
                    ]
                },
                "status": {
                    "description": "Status 'fail'",
                    "type": "string",
                    "example": "fail"
                }
            }
        },
        "JSendFailure-ValidationResponse": {
            "type": "object",
            "properties": {
                "data": {
                    "description": "Data with error message(s)",
                    "allOf": [
                        {
                            "$ref": "#/definitions/ValidationResponse"
                        }
                    ]
                },
                "status": {
                    "description": "Status 'fail'",
                    "type": "string",
                    "example": "fail"
                }
            }
        },
        "JSendSuccess-array_model_User": {
            "type": "object",
            "properties": {
                "data": {
                    "description": "Data with success message(s)",
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/model.User"
                    }
                },
                "status": {
                    "description": "Status 'success'",
                    "type": "string",
                    "example": "success"
                }
            }
        },
        "JSendSuccess-map_string_string": {
            "type": "object",
            "properties": {
                "data": {
                    "description": "Data with success message(s)",
                    "allOf": [
                        {
                            "$ref": "#/definitions/map_string_string"
                        }
                    ]
                },
                "status": {
                    "description": "Status 'success'",
                    "type": "string",
                    "example": "success"
                }
            }
        },
        "JSendSuccess-model_User": {
            "type": "object",
            "properties": {
                "data": {
                    "description": "Data with success message(s)",
                    "allOf": [
                        {
                            "$ref": "#/definitions/model.User"
                        }
                    ]
                },
                "status": {
                    "description": "Status 'success'",
                    "type": "string",
                    "example": "success"
                }
            }
        },
        "PingResp": {
            "type": "object",
            "properties": {
                "message": {
                    "type": "string",
                    "example": "pong"
                }
            }
        },
        "ValidationError": {
            "type": "object",
            "properties": {
                "field": {
                    "description": "Field Query or JSON field",
                    "type": "string",
                    "example": "query_field"
                },
                "reason": {
                    "description": "Validation error reason",
                    "type": "string",
                    "example": "reason"
                }
            }
        },
        "ValidationResponse": {
            "type": "object",
            "properties": {
                "errors": {
                    "description": "Validation errors",
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/ValidationError"
                    }
                }
            }
        },
        "adrcontroller.ADR": {
            "type": "object",
            "properties": {
                "description": {
                    "type": "string"
                },
                "frequency": {
                    "type": "string"
                },
                "frequency_code": {
                    "type": "integer"
                }
            }
        },
        "adrcontroller.PznADR": {
            "type": "object",
            "properties": {
                "adrs": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/adrcontroller.ADR"
                    }
                },
                "pzn": {
                    "type": "string"
                }
            }
        },
        "cfg.LimitsConfig": {
            "description": "Configuration limits for the API",
            "type": "object",
            "properties": {
                "max_batch_queries": {
                    "description": "Max number of baches for POST requests",
                    "type": "integer",
                    "example": 50
                },
                "max_drugs": {
                    "description": "Max number of drugs for interaction check",
                    "type": "integer",
                    "example": 100
                }
            }
        },
        "cfg.MetaConfig": {
            "description": "Meta Information for the API",
            "type": "object",
            "properties": {
                "api": {
                    "type": "string",
                    "example": "API Name"
                },
                "description": {
                    "type": "string",
                    "example": "API Description"
                },
                "url": {
                    "type": "string",
                    "example": "https://api.example.com"
                },
                "version": {
                    "type": "string",
                    "example": "1.0.0"
                },
                "version_tag": {
                    "type": "string",
                    "example": "sometag"
                }
            }
        },
        "map_string_string": {
            "type": "object",
            "additionalProperties": {
                "type": "string"
            }
        },
        "model.User": {
            "type": "object",
            "properties": {
                "email": {
                    "type": "string",
                    "example": "joe@me.com"
                },
                "first_name": {
                    "type": "string",
                    "example": "Joe"
                },
                "last_login": {
                    "type": "string",
                    "example": "2021-01-01T00:00:00Z"
                },
                "last_name": {
                    "type": "string",
                    "example": "Doe"
                },
                "organization": {
                    "type": "string",
                    "example": "ACME"
                },
                "role": {
                    "type": "string",
                    "example": "admin"
                },
                "status": {
                    "type": "string",
                    "example": "active"
                }
            }
        }
    },
    "securityDefinitions": {
        "Bearer": {
            "description": "Type \"Bearer\" followed by a space and JWT token.",
            "type": "apiKey",
            "name": "Authorization",
            "in": "header"
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "",
	Host:             "",
	BasePath:         "",
	Schemes:          []string{},
	Title:            "",
	Description:      "",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
