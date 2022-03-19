package downloader

import "testing"

func TestDry(t *testing.T) {
	j := []Job[any]{
		{Tag: 1, SaveFilePath: "", Url: ""},
	}
	_, cancelFunc := NewMultiDownloader(ConfigRaw{
		SharedConfig: SharedConfig{
			ShowProgress:        false,
			SkipSameLength:      false,
			Workers:             1,
			Jobs:                j,
			ExpectedStatusCodes: []int{},
		},
	}).Run()
	cancelFunc()
}
