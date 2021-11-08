package packaging

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/pkg/content"
	"oras.land/oras-go/pkg/oras"
)

const (
	configMediaType = "application/ebpf.oci.image.config.v1+json"
	eBPFMediaType   = "binary/ebpf.solo.io.v1"

	ebpfFileName = "program.o"
	configName   = "config.json"
)

type EbpfPackage struct {
	// File content for eBPF compiled ELF file
	ProgramFileBytes []byte

	EbpfConfig
}

type EbpfConfig struct {
	Info string `json:"info"`
}

type EbpfRegistry interface {
	Push(ctx context.Context, ref string, pkg *EbpfPackage) error
	Pull(ctx context.Context, ref string) (*EbpfPackage, error)
}

func NewEbpfRegistry(
	registry *content.Registry,
) EbpfRegistry {
	return &ebpfResgistry{
		registry: registry,
	}
}

type ebpfResgistry struct {
	registry *content.Registry
}

func (e *ebpfResgistry) Push(ctx context.Context, ref string, pkg *EbpfPackage) error {

	memoryStore := content.NewMemory()

	progDesc, err := memoryStore.Add(ebpfFileName, eBPFMediaType, pkg.ProgramFileBytes)
	if err != nil {
		return err
	}

	configByt, err := json.Marshal(pkg.EbpfConfig)
	if err != nil {
		return err
	}

	configDesc, err := buildConfigDescriptor(configByt, nil)
	if err != nil {
		return err
	}

	memoryStore.Set(configDesc, configByt)

	manifest, manifestDesc, err := content.GenerateManifest(&configDesc, nil, progDesc)
	if err != nil {
		return err
	}

	err = memoryStore.StoreManifest(ref, manifestDesc, manifest)
	if err != nil {
		return err
	}

	_, err = oras.Copy(ctx, memoryStore, ref, e.registry, "")
	return err
}

func (e *ebpfResgistry) Pull(ctx context.Context, ref string) (*EbpfPackage, error) {
	memoryStore := content.NewMemory()
	_, err := oras.Copy(ctx, e.registry, ref, memoryStore, "")
	if err != nil {
		return nil, err
	}

	_, ebpfBytes, ok := memoryStore.GetByName(ebpfFileName)
	if !ok {
		return nil, errors.New("could not find ebpf bytes in manifest")
	}

	_, configBytes, ok := memoryStore.GetByName(configName)
	if !ok {
		return nil, errors.New("could not find ebpf bytes in manifest")
	}

	var cfg EbpfConfig
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil, err
	}

	return &EbpfPackage{
		ProgramFileBytes: ebpfBytes,
		EbpfConfig:       cfg,
	}, nil
}

// GenerateConfig generates a blank config with optional annotations.
func buildConfigDescriptor(byt []byte, annotations map[string]string) (ocispec.Descriptor, error) {
	dig := digest.FromBytes(byt)
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[ocispec.AnnotationTitle] = configName
	config := ocispec.Descriptor{
		MediaType:   configMediaType,
		Digest:      dig,
		Size:        int64(len(byt)),
		Annotations: annotations,
	}
	return config, nil
}
