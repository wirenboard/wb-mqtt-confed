import json
import sys

from network_managing_system import ParseError
from network_interfaces_adapter import NetworkInterfacesAdapter
from network_manager_adapter import NetworkManagerAdapter

JSON_INDENT_LEVEL = 2
SERIAL_FILE = "/var/lib/wirenboard/serial.conf"
PRIMARY_IFACE = "eth0"

_cached_serial = None


def get_adapters():
    adapters = []
    nm = NetworkManagerAdapter.probe()
    if nm is not None:
        adapters.append(nm)
    ni = NetworkInterfacesAdapter.probe()
    if ni is not None:
        adapters.append(ni)
    return adapters


def get_serial():
    global _cached_serial
    if _cached_serial is None:
        try:
            with open(SERIAL_FILE, "r") as f:
                _cached_serial = (f.readline().strip(),)
        except IOError:
            print >>sys.stderr, "cannot find %s" % SERIAL_FILE
            _cached_serial = (None,)
    return _cached_serial[0]


def to_json():
    interfaces = []
    for adapter in get_adapters():
        interfaces = interfaces + adapter.read()
    serial = get_serial()
    if serial is not None:
        for iface in interfaces:
            if iface.get("name") == PRIMARY_IFACE:
                iface.setdefault("options", {})["hwaddress"] = serial
                break
    r = dict(interfaces=interfaces)
    json.dump(r, sys.stdout, sort_keys=True, indent=JSON_INDENT_LEVEL)


def from_json():
    try:
        d = json.load(sys.stdin)
    except ValueError:
        print >>sys.stdout, "Invalid JSON"
        sys.exit(1)

    interfaces = d["interfaces"]
    for adapter in get_adapters():
        interfaces = adapter.apply(interfaces)


def main():
    if len(sys.argv) > 1 and sys.argv[1] == "-s":
        from_json()
    else:
        to_json()


if __name__ == "__main__":
    main()
