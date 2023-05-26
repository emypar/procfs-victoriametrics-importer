#! /usr/bin/env python3

# support for pvmi/proc_net_snmp_snmp6_metrics.go testing

from typing import List, Optional

import procfs
from metrics_common_test import TestHostname, TestJob

from .common import Metric, ts_to_prometheus_ts

net_snmp_metric_name_map = {
    "Icmp:InAddrMaskReps": "net_snmp_icmp_in_addr_mask_reps_total",
    "Icmp:InAddrMasks": "net_snmp_icmp_in_addr_masks_total",
    "Icmp:InCsumErrors": "net_snmp_icmp_in_csum_errors_total",
    "Icmp:InDestUnreachs": "net_snmp_icmp_in_dest_unreachs_total",
    "Icmp:InEchoReps": "net_snmp_icmp_in_echo_reps_total",
    "Icmp:InEchos": "net_snmp_icmp_in_echos_total",
    "Icmp:InErrors": "net_snmp_icmp_in_errors_total",
    "Icmp:InMsgs": "net_snmp_icmp_in_msgs_total",
    "Icmp:InParmProbs": "net_snmp_icmp_in_parm_probs_total",
    "Icmp:InRedirects": "net_snmp_icmp_in_redirects_total",
    "Icmp:InSrcQuenchs": "net_snmp_icmp_in_src_quenchs_total",
    "Icmp:InTimeExcds": "net_snmp_icmp_in_time_excds_total",
    "Icmp:InTimestampReps": "net_snmp_icmp_in_timestamp_reps_total",
    "Icmp:InTimestamps": "net_snmp_icmp_in_timestamps_total",
    "Icmp:OutAddrMaskReps": "net_snmp_icmp_out_addr_mask_reps_total",
    "Icmp:OutAddrMasks": "net_snmp_icmp_out_addr_masks_total",
    "Icmp:OutDestUnreachs": "net_snmp_icmp_out_dest_unreachs_total",
    "Icmp:OutEchoReps": "net_snmp_icmp_out_echo_reps_total",
    "Icmp:OutEchos": "net_snmp_icmp_out_echos_total",
    "Icmp:OutErrors": "net_snmp_icmp_out_errors_total",
    "Icmp:OutMsgs": "net_snmp_icmp_out_msgs_total",
    "Icmp:OutParmProbs": "net_snmp_icmp_out_parm_probs_total",
    "Icmp:OutRedirects": "net_snmp_icmp_out_redirects_total",
    "Icmp:OutSrcQuenchs": "net_snmp_icmp_out_src_quenchs_total",
    "Icmp:OutTimeExcds": "net_snmp_icmp_out_time_excds_total",
    "Icmp:OutTimestampReps": "net_snmp_icmp_out_timestamp_reps_total",
    "Icmp:OutTimestamps": "net_snmp_icmp_out_timestamps_total",
    "IcmpMsg:InType3": "net_snmp_icmp_msg_in_type3_total",
    "IcmpMsg:OutType3": "net_snmp_icmp_msg_out_type3_total",
    "Ip:DefaultTTL": "net_snmp_ip_default_ttl_total",
    "Ip:ForwDatagrams": "net_snmp_ip_forw_datagrams_total",
    "Ip:Forwarding": "net_snmp_ip_forwarding_total",
    "Ip:FragCreates": "net_snmp_ip_frag_creates_total",
    "Ip:FragFails": "net_snmp_ip_frag_fails_total",
    "Ip:FragOKs": "net_snmp_ip_frag_oks_total",
    "Ip:InAddrErrors": "net_snmp_ip_in_addr_errors_total",
    "Ip:InDelivers": "net_snmp_ip_in_delivers_total",
    "Ip:InDiscards": "net_snmp_ip_in_discards_total",
    "Ip:InHdrErrors": "net_snmp_ip_in_hdr_errors_total",
    "Ip:InReceives": "net_snmp_ip_in_receives_total",
    "Ip:InUnknownProtos": "net_snmp_ip_in_unknown_protos_total",
    "Ip:OutDiscards": "net_snmp_ip_out_discards_total",
    "Ip:OutNoRoutes": "net_snmp_ip_out_no_routes_total",
    "Ip:OutRequests": "net_snmp_ip_out_requests_total",
    "Ip:ReasmFails": "net_snmp_ip_reasm_fails_total",
    "Ip:ReasmOKs": "net_snmp_ip_reasm_oks_total",
    "Ip:ReasmReqds": "net_snmp_ip_reasm_reqds_total",
    "Ip:ReasmTimeout": "net_snmp_ip_reasm_timeout_total",
    "Tcp:ActiveOpens": "net_snmp_tcp_active_opens_total",
    "Tcp:AttemptFails": "net_snmp_tcp_attempt_fails_total",
    "Tcp:CurrEstab": "net_snmp_tcp_curr_estab_total",
    "Tcp:EstabResets": "net_snmp_tcp_estab_resets_total",
    "Tcp:InCsumErrors": "net_snmp_tcp_in_csum_errors_total",
    "Tcp:InErrs": "net_snmp_tcp_in_errs_total",
    "Tcp:InSegs": "net_snmp_tcp_in_segs_total",
    "Tcp:MaxConn": "net_snmp_tcp_max_conn_total",
    "Tcp:OutRsts": "net_snmp_tcp_out_rsts_total",
    "Tcp:OutSegs": "net_snmp_tcp_out_segs_total",
    "Tcp:PassiveOpens": "net_snmp_tcp_passive_opens_total",
    "Tcp:RetransSegs": "net_snmp_tcp_retrans_segs_total",
    "Tcp:RtoAlgorithm": "net_snmp_tcp_rto_algorithm_total",
    "Tcp:RtoMax": "net_snmp_tcp_rto_max_total",
    "Tcp:RtoMin": "net_snmp_tcp_rto_min_total",
    "Udp:IgnoredMulti": "net_snmp_udp_ignored_multi_total",
    "Udp:InCsumErrors": "net_snmp_udp_in_csum_errors_total",
    "Udp:InDatagrams": "net_snmp_udp_in_datagrams_total",
    "Udp:InErrors": "net_snmp_udp_in_errors_total",
    "Udp:MemErrors": "net_snmp_udp_mem_errors_total",
    "Udp:NoPorts": "net_snmp_udp_no_ports_total",
    "Udp:OutDatagrams": "net_snmp_udp_out_datagrams_total",
    "Udp:RcvbufErrors": "net_snmp_udp_rcvbuf_errors_total",
    "Udp:SndbufErrors": "net_snmp_udp_sndbuf_errors_total",
    "UdpLite:IgnoredMulti": "net_snmp_udp_lite_ignored_multi_total",
    "UdpLite:InCsumErrors": "net_snmp_udp_lite_in_csum_errors_total",
    "UdpLite:InDatagrams": "net_snmp_udp_lite_in_datagrams_total",
    "UdpLite:InErrors": "net_snmp_udp_lite_in_errors_total",
    "UdpLite:MemErrors": "net_snmp_udp_lite_mem_errors_total",
    "UdpLite:NoPorts": "net_snmp_udp_lite_no_ports_total",
    "UdpLite:OutDatagrams": "net_snmp_udp_lite_out_datagrams_total",
    "UdpLite:RcvbufErrors": "net_snmp_udp_lite_rcvbuf_errors_total",
    "UdpLite:SndbufErrors": "net_snmp_udp_lite_sndbuf_errors_total",
}

net_snmp6_metric_name_map = {
    "Icmp6InCsumErrors": "net_snmp6_icmp6_in_csum_errors_total",
    "Icmp6InDestUnreachs": "net_snmp6_icmp6_in_dest_unreachs_total",
    "Icmp6InEchoReplies": "net_snmp6_icmp6_in_echo_replies_total",
    "Icmp6InEchos": "net_snmp6_icmp6_in_echos_total",
    "Icmp6InErrors": "net_snmp6_icmp6_in_errors_total",
    "Icmp6InGroupMembQueries": "net_snmp6_icmp6_in_group_memb_queries_total",
    "Icmp6InGroupMembReductions": "net_snmp6_icmp6_in_group_memb_reductions_total",
    "Icmp6InGroupMembResponses": "net_snmp6_icmp6_in_group_memb_responses_total",
    "Icmp6InMLDv2Reports": "net_snmp6_icmp6_in_mldv2_reports_total",
    "Icmp6InMsgs": "net_snmp6_icmp6_in_msgs_total",
    "Icmp6InNeighborAdvertisements": "net_snmp6_icmp6_in_neighbor_advertisements_total",
    "Icmp6InNeighborSolicits": "net_snmp6_icmp6_in_neighbor_solicits_total",
    "Icmp6InParmProblems": "net_snmp6_icmp6_in_parm_problems_total",
    "Icmp6InPktTooBigs": "net_snmp6_icmp6_in_pkt_too_bigs_total",
    "Icmp6InRedirects": "net_snmp6_icmp6_in_redirects_total",
    "Icmp6InRouterAdvertisements": "net_snmp6_icmp6_in_router_advertisements_total",
    "Icmp6InRouterSolicits": "net_snmp6_icmp6_in_router_solicits_total",
    "Icmp6InTimeExcds": "net_snmp6_icmp6_in_time_excds_total",
    "Icmp6OutDestUnreachs": "net_snmp6_icmp6_out_dest_unreachs_total",
    "Icmp6OutEchoReplies": "net_snmp6_icmp6_out_echo_replies_total",
    "Icmp6OutEchos": "net_snmp6_icmp6_out_echos_total",
    "Icmp6OutErrors": "net_snmp6_icmp6_out_errors_total",
    "Icmp6OutGroupMembQueries": "net_snmp6_icmp6_out_group_memb_queries_total",
    "Icmp6OutGroupMembReductions": "net_snmp6_icmp6_out_group_memb_reductions_total",
    "Icmp6OutGroupMembResponses": "net_snmp6_icmp6_out_group_memb_responses_total",
    "Icmp6OutMLDv2Reports": "net_snmp6_icmp6_out_mldv2_reports_total",
    "Icmp6OutMsgs": "net_snmp6_icmp6_out_msgs_total",
    "Icmp6OutNeighborAdvertisements": "net_snmp6_icmp6_out_neighbor_advertisements_total",
    "Icmp6OutNeighborSolicits": "net_snmp6_icmp6_out_neighbor_solicits_total",
    "Icmp6OutParmProblems": "net_snmp6_icmp6_out_parm_problems_total",
    "Icmp6OutPktTooBigs": "net_snmp6_icmp6_out_pkt_too_bigs_total",
    "Icmp6OutRedirects": "net_snmp6_icmp6_out_redirects_total",
    "Icmp6OutRouterAdvertisements": "net_snmp6_icmp6_out_router_advertisements_total",
    "Icmp6OutRouterSolicits": "net_snmp6_icmp6_out_router_solicits_total",
    "Icmp6OutTimeExcds": "net_snmp6_icmp6_out_time_excds_total",
    "Icmp6OutType133": "net_snmp6_icmp6_out_type133_total",
    "Icmp6OutType135": "net_snmp6_icmp6_out_type135_total",
    "Icmp6OutType143": "net_snmp6_icmp6_out_type143_total",
    "Ip6FragCreates": "net_snmp6_ip6_frag_creates_total",
    "Ip6FragFails": "net_snmp6_ip6_frag_fails_total",
    "Ip6FragOKs": "net_snmp6_ip6_frag_oks_total",
    "Ip6InAddrErrors": "net_snmp6_ip6_in_addr_errors_total",
    "Ip6InBcastOctets": "net_snmp6_ip6_in_bcast_octets_total",
    "Ip6InCEPkts": "net_snmp6_ip6_in_cepkts_total",
    "Ip6InDelivers": "net_snmp6_ip6_in_delivers_total",
    "Ip6InDiscards": "net_snmp6_ip6_in_discards_total",
    "Ip6InECT0Pkts": "net_snmp6_ip6_in_ect0_pkts_total",
    "Ip6InECT1Pkts": "net_snmp6_ip6_in_ect1_pkts_total",
    "Ip6InHdrErrors": "net_snmp6_ip6_in_hdr_errors_total",
    "Ip6InMcastOctets": "net_snmp6_ip6_in_mcast_octets_total",
    "Ip6InMcastPkts": "net_snmp6_ip6_in_mcast_pkts_total",
    "Ip6InNoECTPkts": "net_snmp6_ip6_in_no_ectpkts_total",
    "Ip6InNoRoutes": "net_snmp6_ip6_in_no_routes_total",
    "Ip6InOctets": "net_snmp6_ip6_in_octets_total",
    "Ip6InReceives": "net_snmp6_ip6_in_receives_total",
    "Ip6InTooBigErrors": "net_snmp6_ip6_in_too_big_errors_total",
    "Ip6InTruncatedPkts": "net_snmp6_ip6_in_truncated_pkts_total",
    "Ip6InUnknownProtos": "net_snmp6_ip6_in_unknown_protos_total",
    "Ip6OutBcastOctets": "net_snmp6_ip6_out_bcast_octets_total",
    "Ip6OutDiscards": "net_snmp6_ip6_out_discards_total",
    "Ip6OutForwDatagrams": "net_snmp6_ip6_out_forw_datagrams_total",
    "Ip6OutMcastOctets": "net_snmp6_ip6_out_mcast_octets_total",
    "Ip6OutMcastPkts": "net_snmp6_ip6_out_mcast_pkts_total",
    "Ip6OutNoRoutes": "net_snmp6_ip6_out_no_routes_total",
    "Ip6OutOctets": "net_snmp6_ip6_out_octets_total",
    "Ip6OutRequests": "net_snmp6_ip6_out_requests_total",
    "Ip6ReasmFails": "net_snmp6_ip6_reasm_fails_total",
    "Ip6ReasmOKs": "net_snmp6_ip6_reasm_oks_total",
    "Ip6ReasmReqds": "net_snmp6_ip6_reasm_reqds_total",
    "Ip6ReasmTimeout": "net_snmp6_ip6_reasm_timeout_total",
    "Udp6IgnoredMulti": "net_snmp6_udp6_ignored_multi_total",
    "Udp6InCsumErrors": "net_snmp6_udp6_in_csum_errors_total",
    "Udp6InDatagrams": "net_snmp6_udp6_in_datagrams_total",
    "Udp6InErrors": "net_snmp6_udp6_in_errors_total",
    "Udp6MemErrors": "net_snmp6_udp6_mem_errors_total",
    "Udp6NoPorts": "net_snmp6_udp6_no_ports_total",
    "Udp6OutDatagrams": "net_snmp6_udp6_out_datagrams_total",
    "Udp6RcvbufErrors": "net_snmp6_udp6_rcvbuf_errors_total",
    "Udp6SndbufErrors": "net_snmp6_udp6_sndbuf_errors_total",
    "UdpLite6InCsumErrors": "net_snmp6_udp_lite6_in_csum_errors_total",
    "UdpLite6InDatagrams": "net_snmp6_udp_lite6_in_datagrams_total",
    "UdpLite6InErrors": "net_snmp6_udp_lite6_in_errors_total",
    "UdpLite6MemErrors": "net_snmp6_udp_lite6_mem_errors_total",
    "UdpLite6NoPorts": "net_snmp6_udp_lite6_no_ports_total",
    "UdpLite6OutDatagrams": "net_snmp6_udp_lite6_out_datagrams_total",
    "UdpLite6RcvbufErrors": "net_snmp6_udp_lite6_rcvbuf_errors_total",
    "UdpLite6SndbufErrors": "net_snmp6_udp_lite6_sndbuf_errors_total",
}


def proc_net_snmp_metrics(
    net_snmp: procfs.FixedLayoutDataModel,
    ts: Optional[float] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> List[Metric]:
    if ts is None:
        ts = net_snmp._ts
    prom_ts = ts_to_prometheus_ts(ts)
    metrics = []
    for i, name in enumerate(net_snmp.Names):
        metric_name = net_snmp_metric_name_map.get(name)
        if metric_name:
            metrics.append(
                Metric(
                    metric=f'{metric_name}{{hostname="{_hostname}",job="{_job}"}}',
                    val=net_snmp.Values[i],
                    ts=prom_ts,
                )
            )
    return metrics


def proc_net_snmp6_metrics(
    net_snmp6: procfs.FixedLayoutDataModel,
    ts: Optional[float] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> List[Metric]:
    if ts is None:
        ts = net_snmp6._ts
    prom_ts = ts_to_prometheus_ts(ts)
    metrics = []
    for i, name in enumerate(net_snmp6.Names):
        metric_name = net_snmp6_metric_name_map.get(name)
        if metric_name:
            metrics.append(
                Metric(
                    metric=f'{metric_name}{{hostname="{_hostname}",job="{_job}"}}',
                    val=net_snmp6.Values[i],
                    ts=prom_ts,
                )
            )
    return metrics
