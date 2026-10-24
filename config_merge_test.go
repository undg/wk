package main

import (
	"reflect"
	"testing"
)

func TestMergeProjectConfig(t *testing.T) {
	progressEnabled, progressDisabled := true, false
	traditional := repositoryContext{
		Root: "/projects/checkout", MainCheckout: "/projects/checkout",
	}
	bare := repositoryContext{Root: "/projects/wrapper"}

	tests := []struct {
		name       string
		global     GlobalConfig
		repo       RepoConfig
		repository repositoryContext
		want       ProjectConfig
	}{
		{
			name:       "omitted overrides inherit global defaults in traditional clones",
			global:     defaultGlobalConfig(),
			repository: traditional,
			want: ProjectConfig{
				Repository: traditional, ProjectName: "checkout", BaseRef: "origin/main",
				SessionTemplate: "{branch} [{project}]", SessionBackend: backendHerdr, HerdrProgress: true,
			},
		},
		{
			name: "repo values override globals including explicit false",
			global: GlobalConfig{
				DefaultBaseRef: "origin/main", SessionTemplate: "{project}/{branch}",
				SessionBackend: backendTmux, HerdrProgress: true,
			},
			repo: RepoConfig{
				Name: "configured-name", BaseRef: "origin/release", HerdrProgress: &progressDisabled,
				Setup: []string{"install dependencies", "generate files"}, Teardown: []string{"remove cache"},
			},
			repository: traditional,
			want: ProjectConfig{
				Repository: traditional, ProjectName: "configured-name", BaseRef: "origin/release",
				SessionTemplate: "{project}/{branch}", SessionBackend: backendTmux, HerdrProgress: false,
				Setup: []string{"install dependencies", "generate files"}, Teardown: []string{"remove cache"},
			},
		},
		{
			name: "bare repository uses root name and repo can enable progress",
			global: GlobalConfig{
				DefaultBaseRef: "upstream/trunk", SessionBackend: backendHerdr, HerdrProgress: false,
			},
			repo:       RepoConfig{HerdrProgress: &progressEnabled},
			repository: bare,
			want: ProjectConfig{
				Repository: bare, ProjectName: "wrapper", BaseRef: "upstream/trunk",
				SessionTemplate: "{branch} [{project}]", SessionBackend: backendHerdr, HerdrProgress: true,
			},
		},
		{
			name: "omitted progress preserves false without inventing base or backend defaults",
			global: GlobalConfig{
				DefaultBaseRef: "", SessionTemplate: "", SessionBackend: "", HerdrProgress: false,
			},
			repository: traditional,
			want: ProjectConfig{
				Repository: traditional, ProjectName: "checkout", BaseRef: "",
				SessionTemplate: "{branch} [{project}]", SessionBackend: "", HerdrProgress: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeProjectConfig(tt.global, tt.repo, tt.repository)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("merged config = %+v, want %+v", got, tt.want)
			}
		})
	}
}
