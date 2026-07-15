package ports

const (
	RTSPPortKey      = "RTSP_PORT"
	RTMPPortKey      = "RTMP_PORT"
	HLSPortKey       = "HLS_PORT"
	WebRTCSignalKey  = "HLS_PORT2"
	SRTPortKey       = "HLS_PORT3"
	WebRTCICEPortKey = "WEBRTC_ICE_PORT"
)

// MigrationRTSPExemptions returns the Redis and MediaMTX ports that may remain
// occupied while an existing installation is under an active migration lease.
func MigrationRTSPExemptions() (map[int]struct{}, map[int]struct{}) {
	return map[int]struct{}{
			6379: {},
			8554: {},
			1935: {},
			8888: {},
			8889: {},
		}, map[int]struct{}{
			8890: {},
			8189: {},
		}
}
