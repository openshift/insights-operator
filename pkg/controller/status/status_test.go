package status

import (
	"reflect"
	"testing"
)

func Test_controllerStatus_getStatus(t *testing.T) {
	type fields struct {
		statusMap map[statusID]statusMessage
	}
	type args struct {
		id statusID
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *statusMessage
	}{
		{
			name:   "Must get nil if there is no status",
			fields: fields{statusMap: map[statusID]statusMessage{}},
			args:   args{id: DisabledStatus},
			want:   nil,
		},
		{
			name: "Can get the status message",
			fields: fields{
				statusMap: map[statusID]statusMessage{
					DisabledStatus: {reason: "disabled reason", message: "disabled message"},
					UploadStatus:   {reason: "upload reason", message: "upload message"},
					DownloadStatus: {reason: "download reason", message: "download message"},
					ErrorStatus:    {reason: "error reason", message: "error message"},
				},
			},
			args: args{id: DownloadStatus},
			want: &statusMessage{"download reason", "download message"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &controllerStatus{
				statusMap: tt.fields.statusMap,
			}
			if got := c.getStatus(tt.args.id); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_controllerStatus_hasStatus(t *testing.T) {
	type fields struct {
		statusMap map[statusID]statusMessage
	}
	type args struct {
		id statusID
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name:   "Must be false if status doesn't exist",
			fields: fields{statusMap: map[statusID]statusMessage{}},
			args:   args{id: DisabledStatus},
			want:   false,
		},
		{
			name: "Must be true if status exists",
			fields: fields{
				statusMap: map[statusID]statusMessage{
					DisabledStatus: {reason: "disabled reason", message: "disabled message"},
					UploadStatus:   {reason: "upload reason", message: "upload message"},
					DownloadStatus: {reason: "download reason", message: "download message"},
					ErrorStatus:    {reason: "error reason", message: "error message"},
				},
			},
			args: args{id: DownloadStatus},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &controllerStatus{
				statusMap: tt.fields.statusMap,
			}
			if got := c.hasStatus(tt.args.id); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("hasStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_controllerStatus_setStatus(t *testing.T) {
	type fields struct {
		statusMap map[statusID]statusMessage
	}
	type args struct {
		id      statusID
		reason  string
		message string
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		statusID statusID
		want     *controllerStatus
	}{
		{
			name:   "Change not existing status",
			fields: fields{statusMap: map[statusID]statusMessage{}},
			args:   args{id: DisabledStatus, reason: "disabled reason", message: "disabled message"},
			want: &controllerStatus{statusMap: map[statusID]statusMessage{
				DisabledStatus: {reason: "disabled reason", message: "disabled message"},
			}},
		},
		{
			name: "Update existing status with new reason",
			fields: fields{statusMap: map[statusID]statusMessage{
				UploadStatus:   {reason: "upload reason", message: "upload message"},
				DisabledStatus: {reason: "disabled reason", message: "disabled message"},
			}},
			args: args{id: DisabledStatus, reason: "new disabled reason", message: "disabled message"},
			want: &controllerStatus{statusMap: map[statusID]statusMessage{
				UploadStatus:   {reason: "upload reason", message: "upload message"},
				DisabledStatus: {reason: "new disabled reason", message: "disabled message"},
			}},
		},
		{
			name: "Update existing status with new message",
			fields: fields{statusMap: map[statusID]statusMessage{
				UploadStatus:   {reason: "upload reason", message: "upload message"},
				DisabledStatus: {reason: "disabled reason", message: "disabled message"},
			}},
			args: args{id: DisabledStatus, reason: "disabled reason", message: "new disabled message"},
			want: &controllerStatus{statusMap: map[statusID]statusMessage{
				UploadStatus:   {reason: "upload reason", message: "upload message"},
				DisabledStatus: {reason: "disabled reason", message: "new disabled message"},
			}},
		},
		{
			name: "Update existing status with same status message",
			fields: fields{statusMap: map[statusID]statusMessage{
				UploadStatus:   {reason: "upload reason", message: "upload message"},
				DisabledStatus: {reason: "disabled reason", message: "disabled message"},
			}},
			args: args{id: DisabledStatus, reason: "disabled reason", message: "disabled message"},
			want: &controllerStatus{statusMap: map[statusID]statusMessage{
				UploadStatus:   {reason: "upload reason", message: "upload message"},
				DisabledStatus: {reason: "disabled reason", message: "disabled message"},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &controllerStatus{
				statusMap: tt.fields.statusMap,
			}
			c.setStatus(tt.args.id, tt.args.reason, tt.args.message)
			if !reflect.DeepEqual(c, tt.want) {
				t.Errorf("entries() = %v, want %v", c, tt.want)
			}
		})
	}
}

func Test_newControllerStatus(t *testing.T) {
	tests := []struct {
		name string
		want *controllerStatus
	}{
		{name: "Test statusController constructor", want: &controllerStatus{statusMap: make(map[statusID]statusMessage)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newControllerStatus(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newControllerStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}
