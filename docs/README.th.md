# osureq: ขอเพลง osu! จากแชท TikTok LIVE

[English](../README.md) · **ภาษาไทย**

โปรแกรมช่วยสตรีมเมอร์ osu! บน Windows ให้ผู้ชม TikTok LIVE ขอแมพผ่านแชทได้ด้วย `!req <ID หรือลิงก์>`
osureq ตรวจกฎ ดาวน์โหลดแมพ ตรวจไฟล์ แล้วเปิดแมพใน osu! ให้ทีละเพลง พร้อมแดชบอร์ดคุมคิว (ไทย/อังกฤษ) และ overlay สำหรับ OBS
ทั้งหมดอยู่ใน `osureq.exe` ไฟล์เดียว ไม่ต้องติดตั้ง

[![build](https://github.com/Lacrymiraosu/tiktoklivereqsong/actions/workflows/build.yml/badge.svg)](https://github.com/Lacrymiraosu/tiktoklivereqsong/actions/workflows/build.yml)

## ความสามารถ

* รับคำขอจากแชท TikTok ผ่าน TikFinity รองรับทั้ง ID ลิงก์ osu! ทุกรูปแบบ และ**ชื่อเพลง** (`!req freedom dive`)
* ตรวจกฎ: ช่วงดาว (ตั้งแยกตามโหมดได้) ความยาว โหมด สถานะแมพ จำนวนคำขอต่อคน คูลดาวน์ แมพซ้ำ รายชื่อแบน เฉพาะผู้ติดตาม
* ดาวน์โหลดจาก mirror (catboy.best → osu.direct → เว็บ osu! ทางการ) และตรวจ MD5 ให้ตรงกับเวอร์ชันล่าสุดบน osu!
* เปิดแมพเข้า osu! (stable และ lazer) เมื่อกด **ถัดไป** หรือเปิดเองอัตโนมัติเมื่อเพลงก่อนหน้าเล่นจบ (ดูจาก tosu)
* รองรับครบ 4 โหมด: osu!, taiko, catch, mania (แมพ convert ใช้ดาวของโหมดนั้นจาก osu!)
* overlay บอกผู้ชมว่าคำขอเข้าคิวหรือถูกปฏิเสธเพราะอะไร แสดงเพลงที่เล่นอยู่ คิวถัดไป และผลการเล่น
* คำสั่งผู้ชม (`!pos` ดูลำดับคิว, `!np` ดูเพลงที่เล่นอยู่, `!voteskip` โหวตข้ามเพลง) ตอบบน overlay พร้อมรูปโปรไฟล์ TikTok
* คำสั่งม็อด (`!skip`, `!remove`, `!reqon`, `!reqoff`), คิวพิเศษจากของขวัญ, โหมดขอเพลงด้วยของขวัญ, แบนคลิกเดียว, ฟังตัวอย่างเพลง
* แผงทดสอบ: จำลองแชทและของขวัญ โหมดทดลองที่ไม่เปิดเกมจริง และปุ่ม **เช็กก่อนไลฟ์** ที่ตรวจทั้งระบบให้ในครั้งเดียว
* สรุปไลฟ์ (เล่นไปกี่เพลง ผลดีที่สุด ใครขอเยอะสุด) บนแดชบอร์ด และแสดงบน overlay ได้
* สลับ stable / lazer ให้เองตามที่ tosu เห็น และแจ้งเมื่อมี osureq เวอร์ชันใหม่
* ปิด tosu, TikFinity หรือปิดทุกอย่างได้จากแดชบอร์ด

## สิ่งที่ต้องมี

* Windows 10/11 64 บิต
* osu! **stable** หรือ **lazer**
* [**TikFinity Desktop**](https://tikfinity.zerody.one/) เชื่อมกับบัญชี TikTok และเปิดไว้ตอนไลฟ์ (osureq รับแชทผ่าน TikFinity)
* [**tosu**](https://github.com/tosuapp/tosu/releases/latest) เปิดไว้ตอนไลฟ์ (ใช้ดูว่าเพลงเล่นจบหรือยัง และเก็บผลการเล่น)
* OBS หรือ TikTok LIVE Studio ถ้าจะใช้ overlay

## เริ่มใช้งาน

1. ดาวน์โหลด `osureq-<เวอร์ชัน>-windows-x64.zip` จาก [Releases](https://github.com/Lacrymiraosu/tiktoklivereqsong/releases/latest)
   แตกไฟล์ไว้ในโฟลเดอร์ของมันเอง เช่น `C:\osureq\` (โปรแกรมจะสร้าง `config.json`, `queue.json`, `osureq.log` และโฟลเดอร์ `cache\` ไว้ข้าง ๆ)
2. เปิด TikFinity, tosu และ osu!
3. ดับเบิลคลิก `osureq.exe` จะมีหน้าต่าง CMD แสดง log (ปิดหน้าต่างนี้ = ปิดโปรแกรม) และแดชบอร์ดเปิดที่ <http://127.0.0.1:21280/>
4. ไปที่ **ตั้งค่า → osu!** เลือก stable หรือ lazer แล้วตั้งกฎต่าง ๆ ตามต้องการ
5. ถ้าจะใช้ overlay ให้เพิ่ม **Browser Source** ใน OBS เป็น `http://127.0.0.1:21280/overlay` (พื้นหลังโปร่งใส)

ลองทั้งระบบก่อนไลฟ์จริงได้ที่แผง **ทดสอบ** บนแดชบอร์ด

### ผู้ชมพิมพ์อะไรได้บ้าง

```
!req 2096297                                      ← ID ของ diff (ถ้าไม่เจอจะลองเป็น ID ของเซ็ต)
!req https://osu.ppy.sh/beatmapsets/962870#mania/2096297
!req https://osu.ppy.sh/beatmapsets/962870        ← ทั้งเซ็ต: เลือก diff ที่ยากที่สุดที่ผ่านกฎ
!req freedom dive                                 ← ชื่อเพลง: เลือกแมพชื่อตรงที่เล่นเยอะที่สุดที่ผ่านกฎ
!remove                                           ← ลบคำขอล่าสุดของตัวเอง
!pos                                              ← คำขอของฉันอยู่คิวที่เท่าไร
!np                                               ← ตอนนี้เล่นเพลงอะไร ใครขอ
```

ถ้าเปิดโหวตข้ามเพลงไว้ (ตั้งค่า → คำสั่ง) ผู้ชมพิมพ์ `!voteskip` ได้ ม็อดใช้ `!skip` (ข้ามเพลงที่เล่นอยู่), `!remove @ชื่อ`, `!reqon` / `!reqoff` (เปิด/ปิดรับคำขอ) ได้
ชื่อคำสั่งทั้งหมดเปลี่ยนได้ในการตั้งค่า

## การทำงาน

```
แชท TikTok ──► TikFinity ──WebSocket──► osureq ──► osu.ppy.sh (ข้อมูลแมพ, MD5)
                                          │
                                          ├──► catboy.best / osu.direct (ดาวน์โหลด .osz) ──► cache\
                                          ├──► osu! (เปิด .osz หรือคัดลอกเข้า Songs)
          tosu ──WebSocket (สถานะเกม)────►│
                                          └──► แดชบอร์ด + overlay (http://127.0.0.1:21280)
```

1. **รับแชท**: TikFinity ส่งข้อความแชทและของขวัญมาทาง WebSocket (`ws://127.0.0.1:21213/`)
   osureq สนใจเฉพาะข้อความที่ขึ้นต้นด้วยคำสั่ง เช่น `!req`
2. **ตรวจผู้ขอ**: เช็กรายชื่อแบน คิวเต็มหรือยัง จำนวนคำขอต่อคน และคูลดาวน์ โดยไม่ต้องใช้เน็ต
3. **อ่านข้อมูลแมพ**: ดึงหน้าแมพจาก osu.ppy.sh (ไม่ต้องใช้ API key) เพื่อเอาชื่อเพลง ดาว ความยาว โหมด สถานะ และ MD5 ของแต่ละ diff
   ถ้าขอด้วยชื่อเพลง จะค้นใน catboy.best ก่อน แล้วลองแมพที่ชื่อตรงที่สุด (เล่นเยอะสุดก่อน) จนกว่าจะเจอแมพที่ผ่านกฎ
4. **ตรวจกฎแมพ**: ดาว ความยาว โหมด สถานะ แมพซ้ำ และแมพที่เพิ่งเล่นไป
   ถ้าไม่ผ่าน overlay จะขึ้นเหตุผลให้ผู้ชมเห็น เพราะ TikFinity เขียนกลับลงแชท TikTok ไม่ได้
5. **เข้าคิว**: คิวถูกบันทึกใน `queue.json` ปิดเปิดโปรแกรมใหม่คิวก็ยังอยู่ ส่วนคนที่ส่งของขวัญครบจะได้คิวพิเศษ (ถ้าเปิดไว้)
6. **ดาวน์โหลดและตรวจไฟล์**: โหลด `.osz` จาก mirror ตามลำดับที่ตั้งไว้ ทีละแมพ แล้วตรวจ MD5 ของไฟล์ `.osu` ข้างในกับข้อมูลจาก osu!
   ถ้า mirror มีไฟล์เวอร์ชันเก่าจะลองแหล่งถัดไป แมพถัด ๆ ไปจะโหลดล่วงหน้าไว้ให้กดเล่นได้ทันที
7. **เปิดแมพ**: กด **ถัดไป** บนแดชบอร์ดหรือคีย์ลัด osureq จะเปิด `.osz` ให้ osu! นำเข้าแล้วกระโดดไปที่แมพนั้น
8. **โหมดอัตโนมัติ**: osureq ติดตามสถานะเกมจาก tosu เมื่อแมพที่เปิดถูกเล่นจริงแล้วผู้เล่นออกจากหน้าผลคะแนน
   แมพถัดไปจะเปิดเองหลังรอไม่กี่วินาที ผลการเล่น (ความแม่น อันดับ คอมโบ miss pp mods) จะถูกเก็บไว้และขึ้นบน overlay
   ถ้า tosu หลุด โหมดอัตโนมัติจะหยุดรอ ไม่มีการเดาด้วยตัวจับเวลา

เซิร์ฟเวอร์ของ osureq ฟังแค่ `127.0.0.1` เครื่องอื่นในเครือข่ายเข้าไม่ได้ และดาวน์โหลดได้เฉพาะโดเมนที่อนุญาตไว้เท่านั้น

รายละเอียดเชิงลึก (โครงสร้างโค้ด, API, การตั้งค่าทุกตัว, การทดสอบ) อยู่ใน [DEVELOPMENT.md](DEVELOPMENT.md) (ภาษาอังกฤษ)

## วิธี build

ต้องมี [Go](https://go.dev/dl/) 1.23 ขึ้นไปบน Windows (`winget install GoLang.Go`)

```powershell
git clone https://github.com/Lacrymiraosu/tiktoklivereqsong.git
cd tiktoklivereqsong
.\build.ps1 -Version 1.3.7
```

`build.ps1` จะรัน `go vet` และ `go test` ก่อน สร้างไฟล์ resource ของ Windows (ไอคอน ชื่อ เวอร์ชัน) แล้ว build ออกมาเป็น `dist\osureq.exe`
ถ้า PowerShell ไม่ยอมรันสคริปต์ ให้ build เองด้วย:

```powershell
$env:CGO_ENABLED = "0"
go run ./tools/winres -version 1.3.7
go build -trimpath -ldflags "-X main.version=1.3.7" -o dist\osureq.exe .
```

### สำหรับคนที่จะแก้โค้ด

* `main.go` ต่อสายทุกอย่าง โค้ดหลักอยู่ใน `internal/<โมดูล>` แยกหน้าที่ชัดเจน หน้าเว็บอยู่ใน `internal/server/web` และถูกฝังลงใน exe
* รัน `go vet ./...` และ `go test ./...` ก่อนและหลังแก้ทุกครั้ง
* อ่าน [Rules for contributors](DEVELOPMENT.md#rules-for-contributors) ก่อนแก้โค้ด และห้ามกด ถัดไป บนเครื่องจริงโดยไม่เปิดโหมดทดลอง (Dry run)

### Build และ release อัตโนมัติด้วย GitHub Actions

* ทุกครั้งที่ push เข้า `main` หรือเปิด pull request, GitHub จะ vet, test และ build ให้ ไฟล์ exe อยู่ในหน้า Actions 14 วัน
* ปล่อยเวอร์ชันใหม่: อัปเดตเลขเวอร์ชันใน `build.ps1` แล้ว push tag

  ```powershell
  git tag v1.3.7
  git push origin v1.3.7
  ```

  GitHub จะ build ด้วยเลขเวอร์ชันจาก tag แล้วสร้าง Release พร้อม `osureq-<เวอร์ชัน>-windows-x64.zip` (มี exe, `LICENSE` และ `THIRD_PARTY_LICENSES.txt`) และไฟล์ `.sha256`

## เครดิต

ข้อมูลแมพ ภาพปก และตัวอย่างเพลงมาจาก osu! (ppy Pty Ltd) แมพเป็นของผู้สร้างแมพ (mapper) และเพลงเป็นของศิลปินเจ้าของเพลง

* [osu!](https://osu.ppy.sh) โดย ppy Pty Ltd: ตัวเกม
* [TikFinity](https://tikfinity.zerody.one/) โดย zerody: ส่งแชทและของขวัญจาก TikTok LIVE
* [tosu](https://github.com/tosuapp/tosu): อ่านสถานะเกม osu! แบบสด
* Mirror ดาวน์โหลดแมพ: [catboy.best](https://catboy.best) และ [osu.direct](https://osu.direct) ใช้ดาวน์โหลดไฟล์ `.osz` (catboy.best ใช้ค้นหาเพลงด้วย)
* ไลบรารี Go: [coder/websocket](https://github.com/coder/websocket), [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys),
  [tc-hib/winres](https://github.com/tc-hib/winres), [nfnt/resize](https://github.com/nfnt/resize), [golang.org/x/image](https://pkg.go.dev/golang.org/x/image)
* ฟอนต์: [Varela Round](https://fonts.google.com/specimen/Varela+Round) และ [IBM Plex Sans Thai](https://fonts.google.com/specimen/IBM+Plex+Sans+Thai) จาก Google Fonts

osureq เป็นโปรเจกต์ของแฟนเกม ไม่มีส่วนเกี่ยวข้องและไม่ได้รับการรับรองจาก ppy Pty Ltd, TikTok, TikFinity หรือ tosu
osu! เป็นเครื่องหมายการค้าของ ppy Pty Ltd และ TikTok เป็นเครื่องหมายการค้าของเจ้าของ

## License

[MIT](../LICENSE) © 2026 kikinomae ส่วน license ของโค้ดภายนอกที่ถูก build รวมไว้ใน `osureq.exe` (Go standard library,
`golang.org/x/sys` และ `coder/websocket`) อยู่ใน [THIRD_PARTY_LICENSES.txt](../THIRD_PARTY_LICENSES.txt)
