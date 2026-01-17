package types

// OSType 操作系统类型
type OSType string

const (
	OSUnknown OSType = "unknown"
	OSRHEL    OSType = "rhel"   // RHEL/CentOS/Rocky/Alma
	OSDebian  OSType = "debian" // Debian
	OSUbuntu  OSType = "ubuntu" // Ubuntu
)

// ArchType CPU架构类型
type ArchType string

const (
	ArchUnknown ArchType = "unknown"
	ArchAMD64   ArchType = "x86_64"
	ArchARM64   ArchType = "aarch64"
)

// FSType 文件系统类型
type FSType string

const (
	FSTypeExt4 FSType = "ext4"
	FSTypeXFS  FSType = "xfs"
)

// DiskInfo 磁盘信息
type DiskInfo struct {
	Name       string          // 设备名 (如 sda, vdb)
	Size       string          // 大小
	Type       string          // 类型 (disk)
	Partitions []PartitionInfo // 分区列表
}

// PartitionInfo 分区信息
type PartitionInfo struct {
	Name       string // 分区名 (如 sda1)
	Size       string // 大小
	FSType     string // 文件系统类型
	MountPoint string // 挂载点
	UUID       string // UUID
}

// SystemInfo 系统信息
type SystemInfo struct {
	OS            OSType   // 操作系统类型
	Arch          ArchType // CPU架构
	KernelVersion string   // 内核版本
	Is64Bit       bool     // 是否64位
	Memory        int64    // 内存大小 (MB)
	Swap          int64    // Swap大小 (MB)
	InChina       bool     // 是否在中国
	SSHPort       int      // SSH端口
}

// InstallConfig 安装配置
type InstallConfig struct {
	SetupPath string // 安装路径
	InChina   bool   // 是否使用中国镜像
	AutoSwap  bool   // 是否自动创建swap
}

// MountConfig 挂载配置
type MountConfig struct {
	Disk        string // 磁盘设备名
	MountPoint  string // 挂载点
	FSType      FSType // 文件系统类型
	Repartition bool   // 是否重新分区
}

// Progress 进度信息
type Progress struct {
	Step    string  // 当前步骤
	Message string  // 消息
	Percent float64 // 进度百分比 (0-1)
	IsError bool    // 是否错误
	Error   error   // 错误信息
}
