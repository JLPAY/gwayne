package crd

import (
	"testing"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestGetBestCRDVersion(t *testing.T) {
	tests := []struct {
		name     string
		crd      *apiextensions.CustomResourceDefinition
		expected string
	}{
		{
			name: "should select storage version",
			crd: &apiextensions.CustomResourceDefinition{
				Spec: apiextensions.CustomResourceDefinitionSpec{
					Versions: []apiextensions.CustomResourceDefinitionVersion{
						{
							Name:    "v1alpha1",
							Served:  true,
							Storage: false,
						},
						{
							Name:    "v1beta1",
							Served:  true,
							Storage: true, // 存储版本
						},
						{
							Name:    "v1",
							Served:  true,
							Storage: false,
						},
					},
				},
			},
			expected: "v1beta1",
		},
		{
			name: "should select served version when no storage version",
			crd: &apiextensions.CustomResourceDefinition{
				Spec: apiextensions.CustomResourceDefinitionSpec{
					Versions: []apiextensions.CustomResourceDefinitionVersion{
						{
							Name:    "v1alpha1",
							Served:  false,
							Storage: false,
						},
						{
							Name:    "v1beta1",
							Served:  true, // 服务版本
							Storage: false,
						},
						{
							Name:    "v1",
							Served:  true,
							Storage: false,
						},
					},
				},
			},
			expected: "v1beta1",
		},
		{
			name: "should select latest version when no storage or served version",
			crd: &apiextensions.CustomResourceDefinition{
				Spec: apiextensions.CustomResourceDefinitionSpec{
					Versions: []apiextensions.CustomResourceDefinitionVersion{
						{
							Name:    "v1alpha1",
							Served:  false,
							Storage: false,
						},
						{
							Name:    "v1beta1",
							Served:  false,
							Storage: false,
						},
						{
							Name:    "v1", // 最新版本
							Served:  false,
							Storage: false,
						},
					},
				},
			},
			expected: "v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getBestCRDVersion(tt.crd)
			if result == nil {
				t.Errorf("expected version %s, got nil", tt.expected)
				return
			}
			if result.Name != tt.expected {
				t.Errorf("expected version %s, got %s", tt.expected, result.Name)
			}
		})
	}
}

func TestGetBestCRDVersionWithEmptyVersions(t *testing.T) {
	crd := &apiextensions.CustomResourceDefinition{
		Spec: apiextensions.CustomResourceDefinitionSpec{
			Versions: []apiextensions.CustomResourceDefinitionVersion{},
		},
	}

	result := getBestCRDVersion(crd)
	if result != nil {
		t.Errorf("expected nil for empty versions, got %s", result.Name)
	}
}

func TestGetBestCRDVersionWithSingleVersion(t *testing.T) {
	crd := &apiextensions.CustomResourceDefinition{
		Spec: apiextensions.CustomResourceDefinitionSpec{
			Versions: []apiextensions.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
				},
			},
		},
	}

	result := getBestCRDVersion(crd)
	if result == nil {
		t.Error("expected version, got nil")
		return
	}
	if result.Name != "v1" {
		t.Errorf("expected version v1, got %s", result.Name)
	}
}
