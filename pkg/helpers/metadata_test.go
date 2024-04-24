package helpers

import (
	"testing"
)

func TestGetMetadataToStruct(t *testing.T) {
	metadata := map[string]any{
		"key1": "value1",
		"key2": 123,
		"key3": true,
	}

	var str string
	ok, err := GetMetadataToStruct(metadata, "key1", &str)
	if !ok || err != nil {
		t.Errorf("GetMetadataToStruct failed for key1")
	}

	var num int
	ok, err = GetMetadataToStruct(metadata, "key2", &num)
	if !ok || err != nil {
		t.Errorf("GetMetadataToStruct failed for key2")
	}

	var flag bool
	ok, err = GetMetadataToStruct(metadata, "key3", &flag)
	if !ok || err != nil {
		t.Errorf("GetMetadataToStruct failed for key3")
	}

	ok, err = GetMetadataToStruct(metadata, "key4", &str)
	if ok || err != nil {
		t.Errorf("GetMetadataToStruct should have returned false for key4")
	}
}

func TestGetMetadataToStruct_NonexistentKey(t *testing.T) {
	metadata := map[string]any{
		"key1": "value1",
		"key2": 123,
		"key3": true,
	}

	var str string
	ok, err := GetMetadataToStruct(metadata, "key5", &str)
	if ok || err != nil {
		t.Errorf("GetMetadataToStruct should have returned false for key5")
	}
}

func TestGetMetadataToStruct_Marshal_Error(t *testing.T) {

	metadata := map[string]any{
		"key1": "value1",
		"key2": nil,
		"key3": true,
	}

	metadata2 := map[string]any{
		"key1": "value1",
		"key2": 123,
		"key3": true,
	}

	// A cyckle in the metadata forces the json.Marshal to fail
	metadata2["key2"] = metadata
	metadata["key2"] = metadata2

	var str any
	_, err := GetMetadataToStruct(metadata, "key2", &str)
	if err == nil {
		t.Errorf("GetMetadataToStruct should have returned error but got %s", str)
	}
}
