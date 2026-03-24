package service

import "testing"

func TestFirstPartitionName(t *testing.T) {
	tests := []struct {
		name string
		disk string
		want string
	}{
		{
			name: "sata disk",
			disk: "sda",
			want: "sda1",
		},
		{
			name: "nvme disk",
			disk: "nvme0n2",
			want: "nvme0n2p1",
		},
		{
			name: "mmc disk",
			disk: "mmcblk0",
			want: "mmcblk0p1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstPartitionName(tt.disk)
			if got != tt.want {
				t.Errorf("firstPartitionName(%q) = %q, want %q", tt.disk, got, tt.want)
			}
		})
	}
}

func TestFirstPartitionPath(t *testing.T) {
	tests := []struct {
		name string
		disk string
		want string
	}{
		{
			name: "sata partition path",
			disk: "sda",
			want: "/dev/sda1",
		},
		{
			name: "nvme partition path",
			disk: "nvme0n2",
			want: "/dev/nvme0n2p1",
		},
		{
			name: "mmc partition path",
			disk: "mmcblk0",
			want: "/dev/mmcblk0p1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstPartitionPath(tt.disk)
			if got != tt.want {
				t.Errorf("firstPartitionPath(%q) = %q, want %q", tt.disk, got, tt.want)
			}
		})
	}
}
