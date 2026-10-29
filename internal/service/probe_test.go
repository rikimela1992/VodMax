package service

import "testing"

func TestParseProbeTracks(t *testing.T) {
	out := []byte(`{"format":{"duration":"6012.4"},"streams":[
		{"codec_type":"video","codec_name":"h264"},
		{"codec_type":"audio","codec_name":"ac3","channels":6,"tags":{"language":"eng","title":"English 5.1"},"disposition":{"default":1}},
		{"codec_type":"audio","codec_name":"aac","channels":2,"tags":{"language":"ita"}},
		{"codec_type":"subtitle","codec_name":"subrip","tags":{"language":"ita"},"disposition":{"forced":1}},
		{"codec_type":"subtitle","codec_name":"hdmv_pgs_subtitle","tags":{"language":"eng"}},
		{"codec_type":"video","codec_name":"mjpeg","disposition":{"attached_pic":1}}]}`)
	p, err := parseProbe(out)
	if err != nil {
		t.Fatal(err)
	}
	if p.Duration != 6012.4 || p.VideoCodec != "h264" || len(p.Audio) != 2 || len(p.Subtitles) != 2 {
		t.Fatalf("%+v", p)
	}
	if a := p.Audio[1]; a.Index != 1 || a.Language != "ita" || a.Codec != "aac" {
		t.Fatalf("audio: %+v", a)
	}
	if p.preferredAudio() != 1 {
		t.Fatalf("preferred audio = %d, want the Italian track", p.preferredAudio())
	}
	if !p.Subtitles[0].Supported || !p.Subtitles[0].Forced || p.Subtitles[1].Supported {
		t.Fatalf("subtitles: %+v", p.Subtitles)
	}
	p.Audio[1].Language = "fre"
	if p.preferredAudio() != 0 {
		t.Fatal("without Italian the file's default track is preferred")
	}
}
