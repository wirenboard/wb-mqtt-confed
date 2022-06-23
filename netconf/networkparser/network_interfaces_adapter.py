#!/usr/bin/env python
# -*- mode: python; coding: utf-8 -*-
#
# Based on https://github.com/privacyidea/networkparser
# The original license follows:
#
# The MIT License (MIT)
#
# Copyright (c) 2015 Cornelius Koelbel
#
# Permission is hereby granted, free of charge, to any person obtaining a copy of
# this software and associated documentation files (the "Software"), to deal in
# the Software without restriction, including without limitation the rights to
# use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
# the Software, and to permit persons to whom the Software is furnished to do so,
# subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
# FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
# COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
# IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
# CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.


import sys
import codecs

from pyparsing import White, Word, alphanums, CharsNotIn
from pyparsing import Forward, Group, OneOrMore
from pyparsing import pythonStyleComment
from pyparsing import Literal
from pyparsing import Optional, ZeroOrMore
from pyparsing import Regex, SkipTo

from os.path import exists

from network_managing_system import NetworkManagingSystem

NETWORK_INTERFACES_CONFIG = "/etc/network/interfaces"

class NetworkInterfacesAdapter(NetworkManagingSystem):

    interface = Word(alphanums + ":")
    key = Word(alphanums + "-_")
    space = White().suppress()
    value = CharsNotIn("{}\n#")
    line = Regex("^.*$")
    comment = ("#")
    method = Regex("loopback|manual|dhcp|static|ppp|bootp|tunnel|wvdial|ipv4ll")
    stanza = Regex("auto|iface|mapping|allow-hotplug")
    option_key = Regex("bridge_\w*|post-\w*|up|down|pre-\w*|address"
                       "|network|netmask|gateway|broadcast|dns-\w*|scope|"
                       "pointtopoint|metric|hwaddress|mtu|hostname|"
                       "leasehours|leasetime|vendor|client|bootfile|server"
                       "|mode|endpoint|dstaddr|local|ttl|provider|unit"
                       "|options|frame|bitrate|netnum|media|wpa-[\w-]*")
    _eol = Literal("\n").suppress()
    option = Forward()
    option << Group(space
                    + option_key
                    + space
                    + SkipTo(_eol))
    interface_block = Forward()
    interface_block << Group(stanza
                             + space
                             + interface
                             + Optional(
                                 space
                                 + Regex("inet|can")
                                 + method
                                 + Group(ZeroOrMore(option))))

    interface_file = OneOrMore(interface_block).ignore(pythonStyleComment)

    def __init__(self, infile=None, content=None):
        self.filename = None
        self.content = None
        if content:
            self.content = content
        elif infile is not None:
            self.filename = infile
            self._read()
        if self.content is not None:
            self.interfaces = self.get_interfaces()

    def _read(self):
        """
        Reread the contents from the disk
        """
        f = codecs.open(self.filename, "r", "utf-8")
        self.content = f.read()
        f.close()

    def get(self):
        """
        return the grouped config
        """
        if self.filename:
            self._read()
        if len(self.content):
            return self.interface_file.parseString(self.content)
        return []


    def format(self):
        """
        Format the single interfaces e.g. for writing to a file.

        [
            {
              "auto": True,
              "method": "static",
              "options": {
                "address": "1.1.1.1",
                "netmask": "255.255.255.0"
              }
            }
        ]
        results in

        auto eth0
        iface eth0 inet static
          address 1.1.1.1
          netmask 255.255.255.0

        :return: string
        """
        output = ""
        for iface in self.interfaces:
            name = iface["name"]
            if iface.get("auto"):
                output += "auto %s\n" % name
            if iface.get("allow-hotplug"):
                output += "allow-hotplug %s\n" % name
            output += "iface %s %s %s\n" % (name, iface.get("mode", "inet"), iface.get("method", "manual"))
            options = iface.get("options", {})
            for opt_key in sorted(options):
                if options[opt_key] not in ("", None):
                    output += "  %s %s\n" % (opt_key, options[opt_key])
            output += "\n"
        return output

    def get_interfaces(self):
        """
        return the configuration using the following structure

        [
            {
              "name": "eth0",
              "auto": True,
              "method": "static",
              "options": {
                "address": "192.168.1.1",
                "netmask": "255.255.255.0",
                "gateway": "192.168.1.254",
                "dns-nameserver": "1.2.3.4"
              }
            }
        ]

        :return: list
        """
        r = []
        interfaces = {}
        np = self.get()
        for idefinition in np:
            name = idefinition[1]
            if name in interfaces:
                iface = interfaces[name]
            else:
                iface = dict(name=name)
                interfaces[name] = iface
                r.append(iface)
            # auto?
            if idefinition[0] == "auto":
                iface["auto"] = True
            if idefinition[0] == "allow-hotplug":
                iface["allow-hotplug"] = True
            elif idefinition[0] == "iface":
                mode = idefinition[2]
                iface["mode"] = mode
                method = idefinition[3]
                iface["method"] = method
            # check for options
            if len(idefinition) == 5:
                options = {}
                for o in idefinition[4]:
                    options[o[0]] = o[1]
                iface["options"] = options
        return r

    @staticmethod
    def probe():
        if (exists(NETWORK_INTERFACES_CONFIG)):
            return NetworkInterfacesAdapter(NETWORK_INTERFACES_CONFIG)
        return None

    def apply(self, interfaces):
        supported_methods = ["loopback", "dhcp", "static", "can", "manual", "ppp"]
        self.interfaces = filter(lambda i: i.get("method") in supported_methods, interfaces)
        sys.stdout.write(self.format())
        return []

    def read(self):
        return self.interfaces
