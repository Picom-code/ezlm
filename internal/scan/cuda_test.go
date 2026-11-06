package scan

import "testing"

func TestCUDAVersionHeader(t *testing.T) {
	cases := map[string]string{
		"| NVIDIA-SMI 616.92                 KMD Version: 616.92        CUDA UMD Version: 13.4     |": "13.4",
		"| NVIDIA-SMI 570.124.06   Driver Version: 570.124.06   CUDA Version: 12.8     |":             "12.8",
	}
	for in, want := range cases {
		if m := cudaVersionRe.FindStringSubmatch(in); m == nil || m[1] != want {
			t.Errorf("%q: got %v, want %s", in, m, want)
		}
	}
}
