from ipaddress import IPv4Interface
from packaging import version

import dbus

from network_managing_system import NetworkManagingSystem

NM_IFACE_NAME = "org.freedesktop.NetworkManager"
NM_PATH = "/org/freedesktop/NetworkManager"
SETTINGS_IFACE_NAME = NM_IFACE_NAME + ".Settings"
SETTINGS_LIST_PATH = NM_PATH + "/Settings"
PROPS_IFACE_NAME = "org.freedesktop.DBus.Properties"

DEV_TYPES = {
    1: "Ethernet",
    2: "Wi-Fi",
    5: "Bluetooth",
    6: "OLPC",
    7: "WiMAX",
    8: "Modem",
    9: "InfiniBand",
    10: "Bond",
    11: "VLAN",
    12: "ADSL",
    13: "Bridge",
    14: "Generic",
    15: "Team",
    16: "TUN",
    17: "IPTunnel",
    18: "MACVLAN",
    19: "VXLAN",
    20: "Veth",
}


def to_mac_string(mac_array):
    if mac_array is None:
        return None
    return ":".join(map(lambda item: "%0.2X" % item, mac_array))


def to_ascii_string(ar):
    if ar is None:
        return None
    return "".join(map(lambda item: "%c" % item, ar))


def to_mac_list(mac_string):
    if mac_string is None:
        return None
    return dbus.Array(map(lambda item: dbus.Byte(int(item, 16)), mac_string.split(":")))


def to_dbus_byte_array(val):
    return dbus.ByteArray(val.encode('utf-8'))


def get_opt(opt_dict, path, default = None):
    if path is None:
        return opt_dict
    path_items = path.split(".")
    obj = opt_dict
    for i in range(len(path_items) - 1):
        obj = obj.get(path_items[i])
        if obj is None:
            return default
    return obj.get(path_items[len(path_items) - 1], default)


def set(dst, dst_path, src, src_path = None, convert_fn = None):
    value = get_opt(src, src_path)
    if convert_fn is not None:
        value = convert_fn(value)
    dst_path_items = dst_path.split(".")
    last_dst_item = dst_path_items[len(dst_path_items) - 1]
    if type(dst) is dbus.Dictionary:
        obj = dst
        for i in range(len(dst_path_items) - 1):
            obj = obj.setdefault(dst_path_items[i], dbus.Dictionary())
        if value is None:
            if last_dst_item in obj:
                del obj[last_dst_item]
        else:
            obj[last_dst_item] = value
    else:
        if value is not None:
            obj = dst
            for i in range(len(dst_path_items) - 1):
                obj = obj.setdefault(dst_path_items[i], {})
            obj[last_dst_item] = value


def remove_undefined_connections(interfaces, bus, settings):
    uids = [get_opt(i, "uuid") for i in interfaces if get_opt(i, "uuid") is not None]
    handlers = [ EthernetConnection(), WiFiConnection(), PPPConnection() ]
    for c_path in settings.ListConnections():
        c_proxy = bus.get_object(NM_IFACE_NAME, c_path)
        c_obj = dbus.Interface(c_proxy, SETTINGS_IFACE_NAME + ".Connection")
        c_settings = c_obj.GetSettings()
        for handler in handlers:
            if (c_settings["connection"]["uuid"] not in uids) and handler.can_manage(c_settings):
                c_obj.Delete()
                break


def set_ipv4_dbus_options(con, iface):
    # remove deprecated parameter ipv4.addresses
    set(con, "ipv4.addresses", None)
    if get_opt(iface, "ipv4.method", "auto") == "manual":
        set(con, "ipv4.method", "manual")
        set(con, "ipv4.gateway", iface, "ipv4.gateway")
        set(con, "ipv4.route-metric", iface, "ipv4.metric")
        net = IPv4Interface("%s/%s" % (get_opt(iface, "ipv4.address"), get_opt(iface, "ipv4.netmask", "255.255.255.0")))
        parts = net.with_prefixlen.split("/")
        addr = dbus.Dictionary({
            "address": parts[0],
            "prefix": dbus.UInt32(parts[1])
        })
        con["ipv4"]["address-data"] = dbus.Array([addr], signature=dbus.Signature("a{sv}"))
        set(con, "ipv4.dhcp-hostname",  None)
        set(con, "ipv4.dhcp-client-id", None)
    else:
        set(con, "ipv4.method", "auto")
        set(con, "ipv4.dhcp-hostname",  iface, "ipv4.hostname")
        set(con, "ipv4.dhcp-client-id", iface, "ipv4.client")
        set(con, "ipv4.gateway", None)
        set(con, "ipv4.address-data", None)
        set(con, "ipv4.route-metric", None)


def get_ipv4_dbus_options(res, cfg):
    set(res, "ipv4.hostname", cfg, "ipv4.dhcp-hostname")
    set(res, "ipv4.client", cfg, "ipv4.dhcp-client-id")
    set(res, "ipv4.gateway", cfg, "ipv4.gateway")
    set(res, "ipv4.metric", cfg, "ipv4.route-metric")
    set(res, "ipv4.method", cfg, "ipv4.method")
    addr_data = get_opt(cfg, "ipv4.address-data")
    if addr_data is not None and len(addr_data) > 0:
        net = IPv4Interface("%s/%d" % (addr_data[0].get("address"), addr_data[0].get("prefix")))
        parts = net.with_netmask.split("/")
        set(res, "ipv4.address", parts[0])
        set(res, "ipv4.netmask", parts[1])


def set_common_dbus_options(con, iface):
    if iface.get("uuid"):
        set(con, "connection.uuid", iface, "uuid")
    set(con, "connection.interface-name", iface, "name")
    set(con, "connection.autoconnect", iface.get("auto", False))


def get_common_dbus_options(cfg, method):
    res = {
        "method": method,
        "name": cfg["connection"]["interface-name"],
        "auto": cfg["connection"].get("autoconnect", True),
        "uuid": cfg["connection"]["uuid"]
    }
    return res


class EthernetConnection:
    def set_dbus_options(self, con, iface, nm_version):
        set_common_dbus_options(con, iface)
        set(con, "802-3-ethernet.mac-address", iface, "hwaddress", to_mac_list)
        set(con, "802-3-ethernet.mtu", iface, "mtu")
        set_ipv4_dbus_options(con, iface)


    def create(self, iface, settings_iface, nm_version):
        con = dbus.Dictionary()
        self.set_dbus_options(con, iface, nm_version)
        set(con, "connection.id", iface, "name")
        settings_iface.AddConnection(con)


    def can_manage(self, cfg):
        return ((cfg["connection"]["type"] in ["802-3-ethernet"]) and 
                (cfg["connection"].get("interface-name") is not None))


    def read(self, c_obj):
        cfg = c_obj.GetSettings()
        if not self.can_manage(cfg):
            return None
        res = get_common_dbus_options(cfg, "nm_ethernet")
        set(res, "hwaddress", cfg, "802-3-ethernet.mac-address", to_mac_string)
        set(res, "mtu", cfg, "802-3-ethernet.mtu")
        get_ipv4_dbus_options(res, cfg)
        return res


class WiFiConnection:
    def set_dbus_options(self, con, iface, nm_version):
        set_common_dbus_options(con, iface)
        set(con, "802-11-wireless.mac-address", iface, "hwaddress", to_mac_list)
        set(con, "802-11-wireless.ssid", iface, "ssid", to_dbus_byte_array)
        wpa_psk = get_opt(iface, "wpa-psk")
        if wpa_psk is not None:
            set(con, "802-11-wireless-security.key-mgmt", "wpa-psk")
            set(con, "802-11-wireless-security.psk", wpa_psk)
        set_ipv4_dbus_options(con, iface)


    def create(self, iface, settings_iface, nm_version):
        con = dbus.Dictionary()
        self.set_dbus_options(con, iface, nm_version)
        set(con, "connection.id", "%s-%s" % (iface["name"], iface["ssid"]))
        settings_iface.AddConnection(con)


    def can_manage(self, cfg):
        return ((cfg["connection"]["type"] in ["802-11-wireless"]) and 
                (cfg["connection"].get("interface-name") is not None))


    def read(self, c_obj):
        cfg = c_obj.GetSettings()
        if not self.can_manage(cfg):
            return None
        res = get_common_dbus_options(cfg, "nm_wifi")
        set(res, "hwaddress", cfg, "802-11-wireless.mac-address", to_mac_string)
        set(res, "ssid",  cfg, "802-11-wireless.ssid", to_ascii_string)
        set(res, "mtu", cfg, "802-11-wireless.mtu")
        if get_opt(cfg, "802-11-wireless-security.key-mgmt") == "wpa-psk":
            set(res, "wpa-psk", get_opt(c_obj.GetSecrets("802-11-wireless-security"), "802-11-wireless-security.psk"))
        get_ipv4_dbus_options(res, cfg)
        return res


class PPPConnection:
    def set_dbus_options(self, con, iface, nm_version):
        set_common_dbus_options(con, iface)
        if get_opt(iface, "apn"):
            set(con, "gsm.apn", iface, "apn")
            set(con, "gsm.auto-config", None)
        else:
            # gsm.auto-config was implemented in NM 1.22.0
            if version.parse(nm_version) >= version.parse("1.22.0"): 
                set(con, "gsm.auto-config", True)


    def create(self, iface, settings_iface, nm_version):
        con = dbus.Dictionary()
        self.set_dbus_options(con, iface, nm_version)
        set(con, "connection.id", "gsm-%s" % iface["name"])
        set(con, "connection.type", "gsm")
        settings_iface.AddConnection(con)


    def can_manage(self, cfg):
        return ((cfg["connection"]["type"] in ["gsm"]) and
                (cfg["connection"].get("interface-name") is not None))


    def read(self, c_obj):
        cfg = c_obj.GetSettings()
        if not self.can_manage(cfg):
            return None
        res = get_common_dbus_options(cfg, "nm_gsm_ppp")
        set(res, "apn", cfg, "gsm.apn")
        return res


def apply(iface, c_handler, bus, settings, nm_version):
    if iface.get("uuid"):
        for c_path in settings.ListConnections():
            c_proxy = bus.get_object(NM_IFACE_NAME, c_path)
            c_obj = dbus.Interface(c_proxy, SETTINGS_IFACE_NAME + ".Connection")
            c_settings = c_obj.GetSettings()
            if c_settings["connection"]["uuid"] == iface["uuid"]:
                if c_settings["connection"]["interface-name"] == iface["name"]:
                    c_handler.set_dbus_options(c_settings, iface, nm_version)
                    c_obj.Update(c_settings)
                else:
                    c_obj.Delete()
                    c_handler.create(iface, settings, nm_version)
                return
    c_handler.create(iface, settings, nm_version)


class NetworkManagerAdapter(NetworkManagingSystem):

    @staticmethod
    def probe():
        try:
            bus = dbus.SystemBus() 
            obj = bus.get_object(NM_IFACE_NAME, NM_PATH)
            interface = dbus.Interface(obj, PROPS_IFACE_NAME)
            nm_version = interface.Get(NM_IFACE_NAME, "Version")
            return NetworkManagerAdapter(nm_version)
        except:
            return None


    def __init__(self, nm_version):
        self.nm_version = nm_version


    def apply(self, interfaces):
        bus = dbus.SystemBus()
        proxy = bus.get_object(NM_IFACE_NAME, SETTINGS_LIST_PATH)
        settings = dbus.Interface(proxy, SETTINGS_IFACE_NAME)
        remove_undefined_connections(interfaces, bus, settings)
        unmanaged_interfaces = []
        handlers = {
            "nm_ethernet": EthernetConnection(),
            "nm_wifi": WiFiConnection(),
            "nm_gsm_ppp": PPPConnection()
        }
        for iface in interfaces:
            handler = handlers.get(iface["method"])
            if handler is not None:
                apply(iface, handler, bus, settings, self.nm_version)
            else:
                unmanaged_interfaces.append(iface)
        return unmanaged_interfaces


    def read(self):
        bus = dbus.SystemBus()
        proxy = bus.get_object(NM_IFACE_NAME, SETTINGS_LIST_PATH)
        settings = dbus.Interface(proxy, SETTINGS_IFACE_NAME)
        handlers = [ EthernetConnection(), WiFiConnection(), PPPConnection() ]
        res = []
        for path in settings.ListConnections():
            con_proxy = bus.get_object(NM_IFACE_NAME, path)
            c_obj = dbus.Interface(con_proxy, SETTINGS_IFACE_NAME + ".Connection")
            for handler in handlers:
                cfg = handler.read(c_obj)
                if cfg is not None:
                    res.append(cfg)
                    break
        return res
