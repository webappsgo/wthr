package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// textNode is one decoded JSON value that remembers object key order, so the
// text form follows the model's declared field order (AI.md PART 14 "Text Serializer").
type textNode struct {
	kind   byte
	keys   []string
	fields map[string]*textNode
	items  []*textNode
	scalar string
}

const (
	textScalar byte = 's'
	textObject byte = 'o'
	textList   byte = 'l'
)

// decodeTextNode decodes the next JSON value from dec into a textNode.
func decodeTextNode(dec *json.Decoder) (*textNode, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return &textNode{kind: textScalar, scalar: scalarText(token)}, nil
	}
	if delim == '{' {
		node := &textNode{kind: textObject, fields: map[string]*textNode{}}
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, _ := keyToken.(string)
			child, err := decodeTextNode(dec)
			if err != nil {
				return nil, err
			}
			node.keys = append(node.keys, key)
			node.fields[key] = child
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return node, nil
	}
	node := &textNode{kind: textList}
	for dec.More() {
		child, err := decodeTextNode(dec)
		if err != nil {
			return nil, err
		}
		node.items = append(node.items, child)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return node, nil
}

// scalarText renders one JSON scalar for text output.
func scalarText(token json.Token) string {
	switch v := token.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case json.Number:
		return v.String()
	}
	return ""
}

// flattenText appends "key: value" lines for node, joining nested object keys with dots
// and rendering a list of scalars on one line separated by ", ".
func flattenText(out *[]string, prefix string, node *textNode) {
	switch node.kind {
	case textObject:
		for _, key := range node.keys {
			name := key
			if prefix != "" {
				name = prefix + "." + key
			}
			flattenText(out, name, node.fields[key])
		}
	case textList:
		allScalar := true
		for _, item := range node.items {
			if item.kind != textScalar {
				allScalar = false
				break
			}
		}
		if allScalar {
			parts := make([]string, 0, len(node.items))
			for _, item := range node.items {
				parts = append(parts, item.scalar)
			}
			*out = append(*out, prefix+": "+strings.Join(parts, ", "))
			return
		}
		for i, item := range node.items {
			flattenText(out, prefix+"."+strconv.Itoa(i), item)
		}
	default:
		*out = append(*out, prefix+": "+node.scalar)
	}
}

// TextFromData maps a response model to the AI.md PART 14 text form. okMessage is the
// text after "OK:" for objects, and itemsFound formats the "{n} items found" header
// for lists. A scalar model is the bare value.
func TextFromData(data interface{}, okMessage string, itemsFound func(n int) string) string {
	encoded, err := json.Marshal(data)
	if err != nil {
		return "ERROR: " + ErrInternal + "\n"
	}
	dec := json.NewDecoder(bytes.NewReader(encoded))
	dec.UseNumber()
	node, err := decodeTextNode(dec)
	if err != nil && err != io.EOF {
		return "ERROR: " + ErrInternal + "\n"
	}
	if node == nil {
		return "OK: " + okMessage + "\n"
	}
	if data, ok := node.fields["data"]; ok && node.kind == textObject {
		if flag, present := node.fields["ok"]; present && flag.scalar == "true" {
			node = data
		}
	}
	var lines []string
	switch node.kind {
	case textScalar:
		return node.scalar + "\n"
	case textObject:
		header := okMessage
		if message, ok := node.fields["message"]; ok && message.kind == textScalar && message.scalar != "" {
			header = message.scalar
		}
		lines = append(lines, "OK: "+header)
		for _, key := range node.keys {
			if key == "message" && node.fields[key].kind == textScalar {
				continue
			}
			flattenText(&lines, key, node.fields[key])
		}
	case textList:
		lines = append(lines, "OK: "+itemsFound(len(node.items)))
		allScalar := true
		for _, item := range node.items {
			if item.kind != textScalar {
				allScalar = false
			}
		}
		for i, item := range node.items {
			if allScalar {
				lines = append(lines, item.scalar)
				continue
			}
			if i > 0 {
				lines = append(lines, "")
			}
			if item.kind == textObject {
				flattenText(&lines, "", item)
			} else {
				flattenText(&lines, strconv.Itoa(i), item)
			}
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// writeTextData writes a response model in the text form with the request language.
func writeTextData(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	retrieved := Translate(r, "api.text_retrieved")
	text := TextFromData(data, retrieved, func(n int) string {
		return TranslateFormat(r, "api.text_items_found", map[string]string{"n": strconv.Itoa(n)})
	})
	writeText(w, status, "%s", text)
}
