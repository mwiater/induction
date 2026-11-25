// Package imagemeta provides the image metadata appliance.
package imagemeta

import "embed"

// Files contains the embedded image-meta pipeline and configuration assets.
//
//go:embed pipeline.yaml config.yaml
var Files embed.FS
