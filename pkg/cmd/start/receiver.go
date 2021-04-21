package start

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

// NewReceiver is a debug endpoint that allows testing of the status destination.
func NewReceiver() *cobra.Command {
	listen := ":8081"
	cmd := &cobra.Command{
		Use:   "start-receiver",
		Short: "Start a listener that accepts and logs uploaded content",
		RunE: func(cmd *cobra.Command, args []string) error {
			return http.ListenAndServe(listen, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				klog.Infof("Handling %s", req.URL.Path)
				contentType := req.Header.Get("Content-Type")
				if len(contentType) == 0 {
					http.Error(w, "Expected a valid Content-Type", http.StatusBadRequest)
					return
				}
				if auth := req.Header.Get("Authorization"); len(auth) > 0 {
					parts := strings.SplitN(auth, " ", 2)
					klog.Infof("Authorization type = %s", parts[0])
				}
				r, err := req.MultipartReader()
				if err != nil {
					http.Error(w, fmt.Sprintf("Expected a valid multipart request: %v", err), http.StatusBadRequest)
					return
				}
				for {
					part, err := r.NextPart()
					if err != nil {
						if err == io.EOF {
							break
						}
						http.Error(w, fmt.Sprintf("Expected a valid multipart request: %v", err), http.StatusBadRequest)
						return
					}
					if part.FormName() != "file" {
						http.Error(w, fmt.Sprintf("Unrecognized form-data field: %s", part.FormName()), http.StatusBadRequest)
						return
					}
					contentType := part.Header.Get("Content-Type")
					if !strings.HasSuffix(contentType, "+tgz") {
						http.Error(w, fmt.Sprintf("Unrecognized part content-type: %s", contentType), http.StatusBadRequest)
						return
					}
					klog.Infof("Got file with content type %s", contentType)
					gr, err := gzip.NewReader(part)
					if err != nil {
						http.Error(w, fmt.Sprintf("Unrecognized input object: %v", err), http.StatusBadRequest)
						return
					}
					tr := tar.NewReader(gr)
					for {
						hdr, err := tr.Next()
						if err != nil {
							if err == io.EOF {
								break
							}
							http.Error(w, fmt.Sprintf("Unrecognized tar archive: %v", err), http.StatusBadRequest)
							return
						}
						klog.Infof("Received: %s %7d %s", hdr.ModTime.UTC().Format(time.RFC3339), hdr.Size, hdr.Name)
					}
				}
				fmt.Fprintln(w, "OK")
			}))
		},
	}
	cmd.Flags().StringVar(&listen, "listen", listen, "Address to listen for snapshots on.")
	return cmd
}
