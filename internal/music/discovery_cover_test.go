package music

import "testing"

func TestDiscoveryCoverUpgradesOnlyTrustedQQHost(t *testing.T) {
	for _, sample := range []struct{ input, want string }{
		{"http://y.gtimg.cn/music/cover.jpg", "https://y.gtimg.cn/music/cover.jpg"},
		{"https://y.gtimg.cn/music/cover.jpg", "https://y.gtimg.cn/music/cover.jpg"},
		{"http://127.0.0.1/private", ""},
		{"http://y.gtimg.cn.evil.example/a", ""},
		{"http://user@y.gtimg.cn/a", ""},
		{"http://y.gtimg.cn:80/a", ""},
		{"//y.gtimg.cn/a", ""},
	} {
		if discoveryCover(sample.input) != sample.want {
			t.Fatal("incorrect trusted cover normalization")
		}
	}
}
