# Capturing L-Connect 3 USB traffic (to extend the protocol)

To decode features not yet supported (per-LED RGB on non-SL-Infinity hubs, LCD
streaming, RPM read-back), capture what L-Connect 3 sends on Windows and diff it
against actions you take in the GUI.

## Windows (recommended — that's where L-Connect runs)

1. Install **Wireshark** with the **USBPcap** option checked.
2. Find your controller's USB device. In a terminal:
   ```powershell
   Get-PnpDevice | ? InstanceId -match 'VID_0CF2'
   ```
3. Start Wireshark, choose the **USBPcap** interface that contains the device
   (USBPcap1/2/…; toggle until you see traffic when you move an L-Connect
   slider).
4. Apply a display filter to isolate the controller, e.g.:
   ```
   usb.idVendor == 0x0cf2 || usb.src contains "0cf2"
   ```
   For HID output reports specifically, look at `USB_INTERRUPT out` / control
   `SET_REPORT` transfers; the **leading byte `e0`** marks our reports.
5. **One action per capture.** Set brightness to 50%, stop. Set it to 100%,
   stop. Change effect, stop. Small diffs are far easier to read.
6. Export the bytes (right-click the URB → Copy → … as Hex Stream) and compare.

## Linux

`usbmon` + Wireshark works the same way:

```bash
sudo modprobe usbmon
# find the bus: lsusb | grep 0cf2  -> Bus 003 Device 00X
sudo wireshark -i usbmon3 -k -Y 'usb.transfer_type == 0x01'
```

Or capture raw with `lianctl` itself by adding temporary logging in
`internal/hid/hidraw_linux.go` `Write()` to print every outgoing buffer.

## Decoding tips

- The report ID is always `0xE0`. Index `0x10` is the "control" register page
  (mode/sync/start). `0x20+ch` is per-channel fan speed. `0x30+ch` is per-channel
  color upload.
- Change exactly one slider and diff: the byte that changes is that field.
- Brightness/speed/direction are single bytes in the commit packet — sweep the
  slider 0→100 and tabulate the byte values (they're often non-linear codes,
  like the brightness `0x00/01/02/03/08` table).
- Add new findings to `docs/PROTOCOL.md` and a driver/case in
  `internal/device/`, with a byte-level test in `unihub_test.go`.
