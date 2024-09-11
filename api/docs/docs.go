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
                    },
                    "500": {
                        "description": "Internal server error",
                        "schema": {
                            "$ref": "#/definitions/ErrorResponse"
                        }
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
                    },
                    "500": {
                        "description": "Internal server error",
                        "schema": {
                            "$ref": "#/definitions/ErrorResponse"
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
        "ErrorResponse": {
            "type": "object",
            "properties": {
                "error": {
                    "description": "Error message",
                    "type": "string",
                    "example": "Internal server error"
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
        "PingResp": {
            "type": "object",
            "properties": {
                "message": {
                    "type": "string",
                    "example": "pong"
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
