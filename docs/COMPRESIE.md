# Compresie și estimarea dimensiunii — research și propunere

> Stare: **research făcut 02.09.2026**, propunere de implementare. Nu s-a scris cod.
> Context: [`../CLAUDE.md`](../CLAUDE.md), [`ARHITECTURA.md`](ARHITECTURA.md).

## 1. Regula care nu se negociază: fără compresie cu pierderi

Repackerii de jocuri obțin o parte importantă din câștig **aruncând date**: reencodează coloana
sonoră, șterg limbile pe care nu le vrei, scot texturile de rezoluție mare. Pentru DHS asta e
**interzis**. Migrăm pozele cuiva de la nuntă și documente de lucru; un fișier restaurat trebuie să
fie **identic bit cu bit** cu originalul. Orice tehnică ce nu poate garanta asta nu intră în produs.

Consecință: din trusa FitGirl putem folosi doar partea fără pierderi, care e mai mică decât pare.

## 2. Ce face de fapt FitGirl

Din ce am găsit, stiva e o combinație de:

| Unealtă | Ce face |
|---|---|
| **precomp / reflate** | Găsește fluxuri deja comprimate cu deflate **în interiorul** fișierelor, le decomprimă, iar la refacere le recomprimă bit-identic. Datele brute se comprimă mult mai bine decât cele deja comprimate. |
| **srep** | Deduplicare pe distanță foarte mare — găsește blocuri repetate la distanțe de gigaocteți. |
| **FreeArc / nanozip** | Compresia finală. FreeArc comută automat între LZMA, PPMd, Tornado, GRzip după tipul fișierului, cu filtre REP (repetiții până la 1 GB), DELTA, BCJ (executabile), LZP, DICT. |
| **XDelta** | Reconstruirea arhivelor din diferențe. |
| *reencodare audio, limbi scoase* | **Cu pierderi. Nu ne interesează.** |

Ideea de reținut nu e „un algoritm magic", ci **preprocesarea**: desfaci ce e deja comprimat,
elimini duplicatele, abia apoi comprimi.

## 3. Cât din asta ajută la datele unui om obișnuit

Aici e vestea proastă. Un joc e plin de asseturi necomprimate și de duplicate. Un director personal
nu e. Împărțind pe categorii:

| Ce ai pe disc | Comprimabil? | Ajută precomp? |
|---|---|---|
| Poze JPEG/PNG/HEIC, video MP4/MKV, muzică MP3/FLAC | **Nu.** 0–2% | PNG: da, puțin. JPEG: marginal |
| Arhive `.zip`, `.7z`, `.rar`, pachete deja comprimate | **Nu** | Da, dar te lupți cu tine însuți |
| **Documente Office** `.docx/.xlsx/.pptx` | Nu direct — **sunt containere ZIP** | **Da, semnificativ** |
| **PDF** | Puțin | **Da** — conțin fluxuri deflate |
| Instalatoare `.exe`, `.msi` | Puțin | **Da** — sunt comprimate intern |
| Text, cod, JSON, XML, CSV, jurnale, dumpuri SQL | **Da, foarte mult** (3–10×) | Nu e cazul |
| Imagini de mașini virtuale, `.vmdk`, `.qcow2`, `.wav`, `.bmp`, `.tiff` | **Da, mult** | Nu e cazul |

**Concluzia onestă:** pe un director tipic — poze, filme, muzică, câteva documente — diferența între
„rapid" și „ultra" e adesea **sub 5%**, pentru un timp de 20–50× mai mare. Câștigul real vine din
altă parte: **deduplicare** și **a nu comprima ce e deja comprimat**.

## 4. Ce propunem, concret

### 4.1 Nu un algoritm pe tot pachetul, ci decizie pe clasă de fișier

În loc de `tot → un algoritm`, fișierele se grupează în **blocuri solide pe clasă**:

- clasa **incompresibilă** (poze, video, muzică, arhive) → **stocată**, fără compresie. Zero timp
  pierdut pe 1% câștig.
- clasa **comprimabilă** (text, cod, documente, configurații) → bloc solid comprimat. Solid =
  fișierele mici se comprimă împreună, deci redundanța dintre ele se exploatează.
- clasa **necunoscută** → test rapid de entropie pe primii 256 KB; dacă nu se comprimă, e stocată.

Doar asta, singură, taie de obicei ore din timpul unui backup mare.

### 4.2 Deduplicare la nivel de fișier

Sumele SHA-256 se calculează oricum pentru integritate. Dacă două fișiere au același hash, al doilea
se stochează ca referință. **Practic gratuit** și, în practică, e câștigul cel mai mare pe un
director personal — aceeași poză în trei locuri, aceeași temă descărcată de două ori, backupuri
vechi uitate în `Downloads`.

Deduplicarea pe blocuri, în stil `srep`, e mult mai complexă. Merge în BACKLOG.

### 4.3 Cele trei niveluri pe care le alege userul

| Nivel | Motor | Viteză (per nucleu) | Când îl alegi |
|---|---|---|---|
| **1. Compatibil** | ZIP / deflate | ~50–100 MB/s | Vrei să poți deschide pachetul **pe orice calculator, fără DHS**. Vezi avertismentul de mai jos. |
| **2. Echilibrat** *(implicit)* | zstd, nivel înalt, fereastră lungă | ~10–25 MB/s | Raport apropiat de 7-Zip Ultra, dar de câteva ori mai rapid, iar decomprimarea e de ordine de mărime mai rapidă. |
| **3. Maxim** | LZMA2 (echivalentul „7-Zip Ultra") | ~1–3 MB/s | Ai timp, spațiul e strâmt și ai multe date comprimabile. Pe poze și filme **nu-ți dă nimic**. |

⚠️ **Tensiune reală la nivelul 1.** Motivul serios pentru care merită ZIP e că, dacă DHS se strică
sau nu mai e la îndemână, userul își scoate fișierele cu Explorer sau Ark. Dar pachetul e **criptat
implicit** (D4), iar un ZIP criptat de noi nu se mai deschide cu nimic altceva. Deci ZIP-ul își
merită locul doar dacă acceptăm și un mod **necriptat**, cu avertisment.
**De decis (D7).**

### 4.4 Nivelul „extrem" — verdict

Partea fără pierderi din stiva FitGirl care ne-ar folosi e **precomp**, și e chiar valoroasă exact
acolo unde ne așteptăm mai puțin: documente Office, PDF-uri și instalatoare. Dar:

- e greu de făcut corect: trebuie să dovedești, pentru fiecare flux, că recompresia iese **bit-identic**;
- implementarea modernă și serioasă e [`microsoft/preflate-rs`](https://github.com/microsoft/preflate-rs),
  scrisă în **Rust** — folosită de Microsoft tocmai pentru stocare unde datele trebuie refăcute exact.
  Nu există echivalent Go matur;
- a lega Rust de un binar Go strică exact avantajul pentru care am ales Go: cross-compile într-o
  comandă și binar static.

**Propunere: nu în v1.** Merge în BACKLOG cu regula, dacă îl facem vreodată: fiecare flux
preprocesat se verifică prin recompresie imediată și, dacă nu iese identic, se stochează originalul.
Fără verificare, nu se atinge.

## 5. Estimarea dimensiunii înainte de backup

Exact scenariul cerut: stick FAT32 de 32 GB, `Downloads` de 12 GB. DHS trebuie să spună **înainte**
dacă încape.

### Cum se estimează

1. **Inventar** (secunde) — parcurge arborele, adună dimensiuni și extensii. Nu citește conținut.
2. **Clasificare** — fiecare fișier primește o clasă și un raport tipic de compresie.
3. **Eșantionare** *(opțional, `--precis`)* — comprimă câțiva MB din fiecare clasă mare, la nivelul
   ales, și înlocuiește raportul presupus cu unul **măsurat**.
4. **Deduplicare** *(la `--precis`)* — hash pe tot; scade duplicatele din total.
5. **Verdict** — interval, nu cifră falsă precisă, plus timp estimat și verificarea spațiului liber.

### Cum arată

```
Sursă        /home/necta                        47,3 GiB   412 883 fișiere
Excluse      cache, node_modules, Steam         18,1 GiB   (editabil)
De inclus                                       29,2 GiB

Nivel        2 · Echilibrat
Duplicate                                       −2,1 GiB
Estimare                                19,4 – 21,8 GiB   ~14 min pe 8 nuclee

Volume       3,5 GiB fiecare                    6 volume
Destinație   /run/media/necta/SSD  (FAT32)      29,8 GiB liberi

             ✓ Încape, cu ~8 GiB de rezervă.
```

Iar când nu încape, DHS nu se oprește cu o eroare — propune, în ordine: exclude ce e mai mare,
urcă un nivel de compresie, sau **împarte pachetul pe mai multe medii**. Formatul pe volume face
împărțirea naturală: volumele 1–6 pe primul stick, 7–11 pe al doilea, iar `dhs.json` știe câte sunt
în total și care lipsesc.

## 6. Volume de 3,5 GiB

FAT32 refuză fișiere de 4 GiB sau mai mari. **3,5 GiB** e implicit peste tot, nu doar pe FAT32 —
uniformitatea e mai valoroasă decât ultimii procenți de eficiență, iar pachetul rămâne mutabil pe
orice mediu, oricând. Se poate schimba explicit acolo unde sistemul de fișiere permite.

Un fișier mai mare decât un volum, de exemplu un ISO de 8 GiB, se **taie între volume**; manifestul
reține că bucățile fac parte din același fișier.

## 7. Paralelizare

Blocurile se comprimă **în paralel pe toate nucleele**, fiindcă la nivelul 3 diferența dintre un
nucleu și opt e între „o oră" și „opt minute". Ordinea de scriere în volume rămâne deterministă,
ca sumele de control să fie reproductibile.

## 8. De implementat, în ordine

1. Inventar + clasificare + estimare rapidă (`dhs scan`)
2. Eșantionare și dedup pentru `--precis`
3. Blocuri solide pe clasă, nivelurile 1 și 2
4. Nivelul 3 (LZMA2)
5. Împărțirea pe mai multe medii

## Surse

- [FitGirl Repacks — Wikipedia](https://en.wikipedia.org/wiki/FitGirl_Repacks)
- [FreeArc — Wikipedia](https://en.wikipedia.org/wiki/FreeArc)
- [Discuții despre metodele repackerilor — FileForums](https://fileforums.com/archive/index.php/t-98094.html)
- [precomp-cpp — recomprimarea fluxurilor deja comprimate](https://github.com/schnaader/precomp-cpp)
- [microsoft/preflate-rs — recompresie deflate fără pierderi](https://github.com/microsoft/preflate-rs)
- [Zstandard — fereastră lungă și deduplicare, ca rzip/lrzip](https://en.wikipedia.org/wiki/Zstd)
- [klauspost/compress — implementarea Go](https://github.com/klauspost/compress)
