package ports

const (
	RTSPPortKey      = "RTSP_PORT"
	RTMPPortKey      = "RTMP_PORT"
	HLSPortKey       = "HLS_PORT"
	WebRTCSignalKey  = "HLS_PORT2"
	SRTPortKey       = "HLS_PORT3"
	WebRTCICEPortKey = "WEBRTC_ICE_PORT"
	MilvusPortKey    = "MILVUS_PORT"
	MilvusWebPortKey = "MILVUS_WEB_PORT"
	MinioAPIPortKey  = "MINIO_API_PORT"
	MinioConsoleKey  = "MINIO_CONSOLE_PORT"
)

// MigrationPortExemptions returns the legacy service ports that may remain
// occupied while an existing installation is under an active migration lease.
func MigrationPortExemptions() (map[int]struct{}, map[int]struct{}) {
	return map[int]struct{}{
			6379:  {},
			8554:  {},
			1935:  {},
			8888:  {},
			8889:  {},
			19530: {},
			9091:  {},
			9000:  {},
			9001:  {},
		}, map[int]struct{}{
			8890: {},
			8189: {},
		}
}
