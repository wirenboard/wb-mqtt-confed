WirenBoard Config Editor Backend
================================

Реализует серверную часть редактора конфигурационных файлов.

Работу с исходными текстами необходимо производить внутри wbdev workspace
(создаётся командой `wbdev update-workspace`).

Сборка исполняемого файла для arm:

```
wbdev hmake clean && wbdev hmake
```

Сборка исполняемого файла для x86_64:

```
wbdev hmake clean && wbdev hmake amd64
```

Сборка пакета для Wiren Board:
```
wbdev gdeb
```
