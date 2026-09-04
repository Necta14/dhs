# Compression and size estimation — research and proposal

> State: research done 2026-09-02; levels 1–3, solid blocks per class and file-level deduplication
> are implemented in `internal/pack`. `--precise` and preprocessing are not.
> Context: [`ARCHITECTURE.md`](ARCHITECTURE.md).

## 1. The non-negotiable rule: no lossy compression

Game repackers get a significant part of their gains by **throwing data away**: they re-encode the
soundtrack, delete the languages you don't want, strip the high-resolution textures. For DHS that is
**forbidden**. We are migrating someone's wedding photos and work documents; a restored file must be
**bit-for-bit identical** to the original. Any technique that cannot guarantee that stays out of the
product.

Consequence: from the FitGirl toolkit we can use only the lossless part, which is smaller than it
looks.

## 2. What FitGirl actually does

From what we found, the stack is a combination of:

| Tool | What it does |
|---|---|
| **precomp / reflate** | Finds streams already compressed with deflate **inside** files, decompresses them, and on reconstruction recompresses them bit-identically. Raw data compresses far better than already-compressed data. |
| **srep** | Very-long-range deduplication — finds repeated blocks gigabytes apart. |
| **FreeArc / nanozip** | The final compression. FreeArc switches automatically between LZMA, PPMd, Tornado, GRzip depending on file type, with REP (repetitions up to 1 GB), DELTA, BCJ (executables), LZP, DICT filters. |
| **XDelta** | Rebuilding archives from differences. |
| *audio re-encoding, removed languages* | **Lossy. Not of interest to us.** |

The thing to remember is not "one magic algorithm" but **preprocessing**: unpack what is already
compressed, remove the duplicates, and only then compress.

## 3. How much of this helps with an ordinary person's data

Here is the bad news. A game is full of uncompressed assets and duplicates. A personal directory is
not. Broken down by category:

| What is on your disk | Compressible? | Does precomp help? |
|---|---|---|
| JPEG/PNG/HEIC photos, MP4/MKV video, MP3/FLAC music | **No.** 0–2% | PNG: yes, a little. JPEG: marginally |
| `.zip`, `.7z`, `.rar` archives, already-compressed packages | **No** | Yes, but you are fighting yourself |
| **Office documents** `.docx/.xlsx/.pptx` | Not directly — **they are ZIP containers** | **Yes, significantly** |
| **PDF** | A little | **Yes** — they contain deflate streams |
| `.exe`, `.msi` installers | A little | **Yes** — they are compressed internally |
| Text, code, JSON, XML, CSV, logs, SQL dumps | **Yes, a lot** (3–10×) | Not applicable |
| Virtual machine images, `.vmdk`, `.qcow2`, `.wav`, `.bmp`, `.tiff` | **Yes, a lot** | Not applicable |

**The honest conclusion:** on a typical directory — photos, films, music, a few documents — the
difference between "fast" and "ultra" is often **under 5%**, for 20–50× the time. The real gain comes
from elsewhere: **deduplication** and **not compressing what is already compressed**.

## 4. What we propose, concretely

### 4.1 Not one algorithm for the whole package, but a decision per file class

Instead of `everything → one algorithm`, files are grouped into **solid blocks per class**:

- the **incompressible** class (photos, video, music, archives) → **stored**, without compression.
  Zero time wasted for a 1% gain.
- the **compressible** class (text, code, documents, configurations) → a compressed solid block.
  Solid = small files are compressed together, so the redundancy between them is exploited.
- the **unknown** class → a quick entropy test on the first 256 KB; if it doesn't compress, it is
  stored.

This alone usually cuts hours off the time of a large backup.

### 4.2 File-level deduplication

SHA-256 checksums are computed anyway for integrity. If two files have the same hash, the second is
stored as a reference. **Practically free** and, in practice, the biggest gain on a personal
directory — the same photo in three places, the same theme downloaded twice, old backups forgotten in
`Downloads`.

Block-level deduplication, `srep` style, is far more complex. It goes in the BACKLOG.

### 4.3 The three levels the user chooses from

| Level | Engine | Speed (per core) | When you choose it |
|---|---|---|---|
| **1 · Compatible** | ZIP / deflate | ~50–100 MB/s | You want to be able to open the package **on any computer, without DHS**. See the warning below. |
| **2 · Balanced** *(default)* | zstd, high level, long window | ~10–25 MB/s | A ratio close to 7-Zip Ultra, but several times faster, and decompression is orders of magnitude faster. |
| **3 · Maximum** | LZMA2 (the "7-Zip Ultra" equivalent) | ~1–3 MB/s | You have time, space is tight and you have a lot of compressible data. On photos and films it **gives you nothing**. |

⚠️ **A real tension at level 1.** The serious reason ZIP is worth having is that, if DHS breaks or
is no longer at hand, the user can get their files out with Explorer or Ark. But the package is
**encrypted by default** (D4), and a ZIP encrypted by us cannot be opened with anything else. So ZIP
earns its place only if we also accept an **unencrypted** mode, with a warning.
**Decided (D7):** level 1 is offered together with `--no-encrypt`, with a visible
warning.

## 4.4 Level 3 and preprocessing — how to do it right

This is the part that must be built carefully, not improvised.

### Safety comes from verification, not avoidance

The natural reflex is "don't touch the important documents". The problem is that **DHS has no way
of knowing which ones are important** — a `.docx` can be a shopping list or the deed to the house.
If safety depends on guessing that, there is no safety.

The correct solution is different: **every preprocessed stream is verified on the spot.**

```
for each candidate stream:
    1. unpack it                                  (deflate → raw data)
    2. recompose it immediately, from what you saved
    3. compare byte by byte with the original
       ├─ identical    → keep the preprocessed form
       └─ different    → throw it all away, store the original untouched
```

A preprocessed stream is **never accepted** without proof that it can be rebuilt exactly. A file we
cannot reconstruct perfectly is simply not preprocessed — it loses a few percent, not its integrity.

On top of that sits the second net: at restore time, the rebuilt file is compared against the
**SHA-256 of the original file**, saved at packing time. We never write to disk a file that does not
match; we report it.

With both checks in place, preprocessing becomes safe **for any file**. Only one, much simpler,
question remains: where is the time worth spending?

### Where it pays off — the allow-list

Preprocessing is expensive, so it is applied only where it is known to pay:

| Type | What is inside | Gain | In v1.1? |
|---|---|---|---|
| `.docx` `.xlsx` `.pptx` `.odt` `.ods` | **ZIP containers** | large | **yes** |
| `.zip` `.jar` `.apk` `.epub` | plain ZIP | large | **yes** |
| `.pdf` | `FlateDecode` objects | moderate | **yes** |
| `.exe` `.msi` installers | internally compressed | variable | yes, with sampling |
| `.png` | zlib in `IDAT` | ~10–20% | **no** — it is an image |
| `.jpg` | lossless transcoding | ~20% | **not in v1** — see below |
| `.mp4` `.mkv` `.mp3` `.flac` | already optimal | ~0 | no |

Below a size threshold (proposal: 64 KiB) nothing is touched — the cost exceeds the gain.

⚠️ **About JPEG, so the decision is informed:** lossless JPEG recompression (`packJPG`, `brunsli`,
JPEG XL) is real and gives ~20%. On a 50 GiB photo collection that means 10 GiB. But these are
exactly the files you can never get back if something goes wrong, and the gain does not justify the
risk in v1. It stays in the BACKLOG, with the figure written here so it can be re-evaluated.

### Measure before you commit

`dhs scan --precise --level 3` samples the real files from the allow-list and says **how much you
actually gain**, not how much you would in theory:

```
Level 2 · Balanced            19.4 – 21.8 GiB     ~14 min
Level 3 · Maximum             17.9 – 19.1 GiB     ~52 min
  of which preprocessing         −1.2 GiB          +31 min   (2 341 files)

  Worth it? 1.4 GiB gained for 38 extra minutes.
```

The user decides with the figure in front of them. No promises, no surprises.

### The implementation problem, stated plainly

For the recompression to come out bit-identical it is not enough to remember "it was deflate level
6" — implementations differ, and Go's `compress/flate` does not produce the same bytes as zlib. A
**recipe** must be saved: the exact encoding decisions of the original stream. That is what
[`microsoft/preflate-rs`](https://github.com/microsoft/preflate-rs) does, written in **Rust** and
used by Microsoft precisely for storage where data must be rebuilt exactly. In Go there is no mature
equivalent.

Two roads, both real:

1. **Reimplementation in Go** of the preflate-style algorithm. Keeps the binary clean and the
   cross-compile a single command. Costs a few weeks of careful work.
2. **`preflate-rs` linked through cgo.** Quick to obtain, but requires `mingw` for compiling to
   Windows and complicates the build chain.

**Not decided now** — it is D8, to be taken when we reach that stage.

### What we do instead, right now

**Level 3 in v1 = LZMA2, without preprocessing.** Functional, useful, shippable.

But the format is designed **from now** so that preprocessing can be added without breaking old
packages: every stored entry has a `preprocessing` field, default `none`. A future version of DHS
writes `preflate/v1` there; an old one sees a value it does not know, says clearly that it needs a
newer version and **breaks nothing**.

This is the "implemented right" part that is done now: not the precomp code, but the **room left
for it**.

## 5. Size estimation before backup

Exactly the scenario asked for: a 32 GB FAT32 stick, a 12 GB `Downloads`. DHS must say
**beforehand** whether it fits.

### How it is estimated

1. **Inventory** (seconds) — walks the tree, gathers sizes and extensions. Does not read content.
2. **Classification** — every file gets a class and a typical compression ratio.
3. **Sampling** *(optional, `--precise`)* — compresses a few MB from every large class, at the
   chosen level, and replaces the assumed ratio with a **measured** one.
4. **Deduplication** *(with `--precise`)* — hashes everything; subtracts the duplicates from the
   total.
5. **Verdict** — a range, not a falsely precise figure, plus estimated time and a check of the free
   space.

### What it looks like

```
Source        /home/you                       45.0 GiB   400 000 files
Excluded      cache, node_modules, Steam      16.0 GiB   (editable)
To include                                    29.0 GiB

Level         2 · Balanced
Duplicates                                    −2.0 GiB
Estimate                              19.0 – 21.5 GiB   ~14 min on 8 cores

Volumes       3.5 GiB each                    6 volumes
Destination   /run/media/you/SSD  (FAT32)     30.0 GiB free

              ✓ Fits, with ~8.5 GiB to spare.
```

And when it does not fit, DHS does not stop with an error — it proposes, in order: exclude what is
largest, go up one compression level, or **split the package across several media**. The volume
format makes the split natural: volumes 1–6 on the first stick, 7–11 on the second, and `dhs.json`
knows how many there are in total and which are missing.

## 6. 3.5 GiB volumes

FAT32 refuses files of 4 GiB or larger. **3.5 GiB** is the default everywhere, not just on FAT32 —
uniformity is worth more than the last few percent of efficiency, and the package stays movable to
any medium, at any time. It can be changed explicitly where the filesystem allows it.

A file larger than a volume, for example an 8 GiB ISO, is **split across volumes**; the manifest
records that the pieces belong to the same file.

## 7. Parallelism

Blocks are compressed **in parallel on all cores**, because at level 3 the difference between one
core and eight is the difference between "an hour" and "eight minutes". The write order into volumes
stays deterministic, so the checksums are reproducible.

## 8. To implement, in order

1. Inventory + classification + quick estimate (`dhs scan`)
2. Sampling and dedup for `--precise`
3. Solid blocks per class, levels 1 and 2
4. Level 3 (LZMA2)
5. Splitting across several media

## Sources

- [FitGirl Repacks — Wikipedia](https://en.wikipedia.org/wiki/FitGirl_Repacks)
- [FreeArc — Wikipedia](https://en.wikipedia.org/wiki/FreeArc)
- [Discussions about repackers' methods — FileForums](https://fileforums.com/archive/index.php/t-98094.html)
- [precomp-cpp — recompressing already-compressed streams](https://github.com/schnaader/precomp-cpp)
- [microsoft/preflate-rs — lossless deflate recompression](https://github.com/microsoft/preflate-rs)
- [Zstandard — long window and deduplication, like rzip/lrzip](https://en.wikipedia.org/wiki/Zstd)
- [klauspost/compress — the Go implementation](https://github.com/klauspost/compress)
