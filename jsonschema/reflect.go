package jsonschema

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Reflect derives a JSON-Schema Definition from the Go type of v using
// reflection. It is intended to produce the input_schema for a tool from a
// typed Go struct, so callers don't have to hand-write schemas.
//
// Supported features:
//   - structs become objects, with one property per exported field
//   - the property name comes from the json tag (falling back to the field name)
//   - a field is required unless it is a pointer, has the json `omitempty`
//     option, or is explicitly marked optional; `jsonschema:"required"` forces
//     a field to be required regardless
//   - basic types (string, bool, integer and float kinds), slices/arrays
//     (as arrays with a typed `items`), maps and nested structs
//   - embedded (anonymous) structs are flattened into the parent object
//   - `time.Time` is treated as a string
//
// The `jsonschema` struct tag accepts a comma-separated list of options:
//
//	`jsonschema:"required,description=The city name,enum=celsius|fahrenheit"`
//
// Note: because options are comma-separated, a description containing a comma
// is not supported via the tag.
//
// For schemas beyond what reflection covers here, build a Definition by hand
// or use a dedicated JSON-schema library and pass the raw schema instead.
func Reflect(v any) (Definition, error) {
	return reflectType(reflect.TypeOf(v))
}

// GenerateSchemaForType is an alias for Reflect, provided for discoverability.
func GenerateSchemaForType(v any) (Definition, error) {
	return Reflect(v)
}

var timeType = reflect.TypeOf(time.Time{})

func reflectType(t reflect.Type) (Definition, error) {
	if t == nil {
		return Definition{}, fmt.Errorf("jsonschema: cannot reflect a nil type")
	}

	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t == timeType {
		return Definition{Type: String}, nil
	}

	switch t.Kind() {
	case reflect.Bool:
		return Definition{Type: Boolean}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Definition{Type: Integer}, nil
	case reflect.Float32, reflect.Float64:
		return Definition{Type: Number}, nil
	case reflect.String:
		return Definition{Type: String}, nil
	case reflect.Slice, reflect.Array:
		item, err := reflectType(t.Elem())
		if err != nil {
			return Definition{}, err
		}
		return Definition{Type: Array, Items: &item}, nil
	case reflect.Map:
		// An object with arbitrary keys; we can't enumerate properties.
		return Definition{Type: Object}, nil
	case reflect.Interface:
		// Unconstrained value; emit an empty (any) schema.
		return Definition{}, nil
	case reflect.Struct:
		return reflectStruct(t)
	default:
		return Definition{}, fmt.Errorf("jsonschema: unsupported type kind %q", t.Kind())
	}
}

func reflectStruct(t reflect.Type) (Definition, error) {
	def := Definition{
		Type:       Object,
		Properties: make(map[string]Definition),
	}
	var required []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" { // unexported
			continue
		}

		name, omitempty := parseJSONTag(field)
		if name == "-" {
			continue
		}

		// Flatten embedded structs that aren't given an explicit json name.
		if field.Anonymous && name == "" {
			ft := field.Type
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				sub, err := reflectType(ft)
				if err != nil {
					return Definition{}, err
				}
				for k, v := range sub.Properties {
					def.Properties[k] = v
				}
				required = append(required, sub.Required...)
				continue
			}
		}

		if name == "" {
			name = field.Name
		}

		propDef, err := reflectType(field.Type)
		if err != nil {
			return Definition{}, err
		}

		opts := parseJSONSchemaTag(field.Tag.Get("jsonschema"))
		if opts.description != "" {
			propDef.Description = opts.description
		}
		if len(opts.enum) > 0 {
			propDef.Enum = opts.enum
		}
		def.Properties[name] = propDef

		isRequired := opts.required || (!omitempty && field.Type.Kind() != reflect.Ptr)
		if isRequired {
			required = append(required, name)
		}
	}

	// Sort for deterministic output (stable schema bytes help prompt caching).
	sort.Strings(required)
	def.Required = required
	return def, nil
}

// parseJSONTag returns the JSON property name and whether omitempty was set.
func parseJSONTag(field reflect.StructField) (name string, omitempty bool) {
	tag := field.Tag.Get("json")
	if tag == "" {
		return "", false
	}
	parts := strings.Split(tag, ",")
	name = parts[0]
	for _, opt := range parts[1:] {
		if opt == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty
}

type jsonSchemaTagOptions struct {
	description string
	enum        []string
	required    bool
}

func parseJSONSchemaTag(tag string) jsonSchemaTagOptions {
	var opts jsonSchemaTagOptions
	if tag == "" {
		return opts
	}
	for _, token := range strings.Split(tag, ",") {
		token = strings.TrimSpace(token)
		switch {
		case token == "required":
			opts.required = true
		case strings.HasPrefix(token, "description="):
			opts.description = strings.TrimPrefix(token, "description=")
		case strings.HasPrefix(token, "enum="):
			opts.enum = strings.Split(strings.TrimPrefix(token, "enum="), "|")
		}
	}
	return opts
}
