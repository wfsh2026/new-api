package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestIsImageGenerationModelRecognizesCodexImageModel(t *testing.T) {
	if !IsImageGenerationModel("gpt-image-2") {
		t.Fatal("gpt-image-2 should be classified as an image generation model")
	}

	endpointTypes := GetEndpointTypesByChannelType(constant.ChannelTypeCodex, "gpt-image-2")
	if len(endpointTypes) == 0 || endpointTypes[0] != constant.EndpointTypeImageGeneration {
		t.Fatalf("gpt-image-2 should prefer the image generation endpoint, got %v", endpointTypes)
	}
}
