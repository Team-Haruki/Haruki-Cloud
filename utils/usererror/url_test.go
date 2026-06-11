package usererror

import "testing"

func TestMessageContainsSensitiveURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{
			name: "production game api",
			in:   "failed from https://production-game-api.sekai.colorfulpalette.org/image/blob/custom-music-score/full/a/b",
			want: true,
		},
		{
			name: "internal ip",
			in:   "toolbox failed: http://100.80.207.86:16666/api/private/game-data/jp/suite/123",
			want: true,
		},
		{
			name: "localhost",
			in:   "cache failed: http://127.0.0.1:6666/cache?path=x",
			want: true,
		},
		{
			name: "docker host",
			in:   "drawing failed: http://haruki-cloud:6666/cache",
			want: true,
		},
		{
			name: "sensitive query",
			in:   "upstream failed: https://docs.example.com/setup?token=secret",
			want: true,
		},
		{
			name: "toolbox public url",
			in:   "工具箱地址：https://haruki.seiunx.com/",
			want: false,
		},
		{
			name: "public tutorial url",
			in:   "教程：https://docs.example.com/help/setup?from=bot",
			want: false,
		},
		{
			name: "public tutorial key parameter",
			in:   "教程：https://docs.example.com/help/setup?key=private-data",
			want: false,
		},
		{
			name: "public github url",
			in:   "参考：https://github.com/team-haruki/example/blob/main/README.md",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MessageContainsSensitiveURL(tt.in); got != tt.want {
				t.Fatalf("MessageContainsSensitiveURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
