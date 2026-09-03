# The licence — options and recommendation (D6)

> State: **proposal**, 2026-09-02, written before the decision. **Decided the same day: path A**
> (Apache-2.0 + trademark policy + `VALUES.md`) — see D6 in [`../CLAUDE.md`](../CLAUDE.md). The
> reasoning is kept in full below.
> Your requirements: a **permissive** licence, but **your rights must stay recognised**, and no
> age-verification-type obligations.
>
> ⚠️ I am not a lawyer; what follows is a technical comparison, not legal advice. For a decision
> with commercial stakes, ask someone qualified.

## First, clearing up age verification

**No open source licence imposes age verification.** Licences regulate who may copy, modify and
redistribute the code — that is all. Age-verification obligations come from entirely different
places:

- **app stores** (Microsoft Store, Google Play, App Store) — they have their own age
  classifications, but those are rating questionnaires, not identity verification;
- **online services where users interact with each other** — forums, chat, user-uploaded content.
  This is where regulations such as the UK Online Safety Act may apply.

DHS is a tool that is downloaded and run locally, with no accounts, no server and no user-generated
content. **It does not touch any of the categories above.** If one day you open an official forum
or Discord, then the question arises — but for that service, not for the licence.

### What does concern you, as a developer in the EU

**Cyber Resilience Act** (Regulation (EU) 2024/2847) — regulates products with digital elements.
The dates that matter:

- **11 September 2026** — the vulnerability-reporting obligations come into force
- **11 December 2027** — the remaining obligations

**Non-commercial** open source software is largely exempt, and the fines do not apply to
non-commercial open source developers. There is also an intermediate category, the "open source
steward", with lighter obligations. If DHS stays free and non-commercial, you are almost certainly
outside the heavy zone. If you ever sell support, a professional edition or the GUI, the situation
changes and it is worth re-reading at that point.

## The licence options

| | MIT | BSD-3-Clause | **Apache-2.0** | MPL-2.0 |
|---|---|---|---|---|
| Permissive | ✅ | ✅ | ✅ | partially |
| Copyright notice must be kept | ✅ | ✅ | ✅ | ✅ |
| Formal attribution mechanism (`NOTICE` file) | ❌ | ❌ | **✅** | ❌ |
| They cannot use your name to promote their fork | ❌ | ✅ | **✅** | ❌ |
| The "DHS" trademark explicitly stays yours | ❌ | partially | **✅** | ❌ |
| Patent grant from contributors | ❌ | ❌ | **✅** | ✅ |
| Whoever sues you over patents loses their licence | ❌ | ❌ | **✅** | ✅ |
| A fork must state what it changed | ❌ | ❌ | **✅** | ✅ |
| Modifications must stay open | ❌ | ❌ | ❌ | ✅ (per file) |
| Length / friction | minimal | minimal | medium | medium |

## Recommendation: Apache-2.0

It is the only permissive licence that gives you, all at once, exactly what you asked for:

1. **Attribution that cannot get lost.** The `NOTICE` file must be carried forward into every
   derivative. It is the most solid "write down who made this" mechanism among the permissive
   licences — MIT and BSD rely only on the copyright notice in the header, which people lose often.
2. **The name stays yours.** Section 6 says explicitly that the licence does **not** grant rights
   to the trademark. Nobody can release a fork and call it "DHS".
3. **Patent protection.** Contributors grant you patent rights over what they bring, and whoever
   sues you over patents automatically loses their licence. MIT and BSD say nothing about patents —
   the ambiguity remains.
4. **Forks must declare what they changed.** So a version broken by someone else cannot be
   confused with yours.
5. It is the standard that serious contributors and corporate legal departments expect.

**The price:** a longer text than MIT and a little extra bureaucracy (`NOTICE`, `LICENSE`, headers).
For a tool that handles people's data and wants contributors, it is worth it.

### If you still want something simpler

- **MIT** — if the priority is maximum adoption and zero friction. You lose the patent grant, the
  trademark clause and `NOTICE`.
- **BSD-3-Clause** — MIT plus "do not use my name to promote your fork". A good compromise, but
  still nothing on patents.

### What "permissive" means, so the choice is an informed one

With Apache-2.0, someone **can** take DHS, close it and sell it. What they **cannot** do: strip the
attribution, call it "DHS", or claim that you endorse it. If that bothers you, then the answer is
not a permissive licence but **GPLv3** — which forces every distributed derivative to stay open.
You said permissive, so I am going with Apache-2.0; I am only flagging what you give up.

## The part that matters most for "my rights"

Not the licence, but **who owns other people's contributions.**

From the moment someone else submits code, **they own that code**. You can no longer change the
licence on your own, and you can no longer do dual commercial licensing without everyone's
consent. Two mechanisms:

| | DCO | CLA |
|---|---|---|
| What it is | the contributor signs `Signed-off-by`, confirming they have the right to give the code | the contributor explicitly grants you broad rights over the contribution |
| Friction | minimal, one line in the commit | a document must be signed |
| Can you relicense later | **no**, not without each person's consent | **yes** |
| Community reaction | good | some contributors refuse on principle |

**Retroactively it is nearly impossible** — you would have to track down every contributor from
three years ago. So the decision is made now, not later.

**Proposal:** DCO to start with. Contributions to a project like this come mostly to the app
database, i.e. data, not code, and the low friction helps growth. Move to a CLA only if a real
intention to sell something appears.

## Modified Apache-2.0 with an ethical clause — the analysis

The requirement: code free to use anywhere, **except** for "evil" purposes and products that do age
verification.

### The central problem: the two halves cancel each other out

You said, in the same sentence, two things that cannot coexist:

> "the code is free to be implemented in other distros/OSes" **and** "as long as it is not used in [X]"

Any restriction on **use** makes the licence **no longer open source**, under both definitions
that matter in practice:

- **Open Source Definition, criterion 6** — "no discrimination against fields of endeavor": the
  licence cannot forbid use in a given field.
- **The Free Software Definition, Freedom 0** — "to run the program as you wish, for any purpose".

And Linux distributions take their policies **directly** from those definitions. The consequence is
concrete, not theoretical: DHS **could no longer enter** Debian, Fedora, the official Arch
repositories, openSUSE, nixpkgs or Homebrew core.

For a tool whose purpose is **to help people move to Linux**, not being packageable in the Linux
distributions is nearly fatal. You would pay that cost, certainly and immediately, to prevent an
almost hypothetical scenario: who would embed a local file-migration tool into an age-verification
product?

### The precedent that has already played out: the JSON licence

Douglas Crockford added a single sentence to an MIT licence: *"The Software shall be used for
Good, not Evil."* What followed, for over twenty years:

- classified **non-free** by Debian, Fedora, Red Hat legal, GNU and the FSF;
- **undistributable** by any organisation that guarantees its users' freedom;
- a cascade effect — whole projects could not be packaged because *one dependency* carried the
  clause;
- legal departments that banned it from the outset, because "evil" is not defined anywhere;
- IBM had to formally request permission to do evil. Crockford granted it, as a joke.

The intention was good. The result was two decades of packaging pain and zero evil prevented.

### And the naming problem

The text of the Apache licence belongs to the Apache Software Foundation. A modified text **can no
longer be called "Apache-2.0"** — it would mislead people and automated tools (SPDX, licence
scanners), which would flag it as a custom, unknown licence anyway. It would have to be given
another name, for instance "DHS Community License 1.0", and then every company that sees it sends
it to legal.

### You are not alone in this camp

There is a whole movement — *Ethical Source* — and ready-made licences, above all the
**Hippocratic License 3.0**, which forbids use in activities that violate the Universal Declaration
of Human Rights. Its authors argue that it respects the open source definition, because the
restrictions target *activities*, not *categories of people*. The OSI has not approved it, and the
distributions treat it as non-free. So your position has company and arguments; it also has the
same practical cost.

### What actually works: the trademark, not the licence

You cannot legally control the **use** of the code without losing open source status. But you can
control very well **your own name** — and the name is what carries the reputation.

Apache-2.0 §6 already reserves the trademark for you. On top of that you publish a **trademark
policy**:

> The code is free, under Apache-2.0. The names "DHS", "Direct Handoff Suite" and the logo are,
> however, my trademarks. You may use them only for unmodified versions. If you embed this code in a
> product that implements age verification, you have no right to call it DHS, to use the logo or to
> say "powered by DHS". Rename it.

The difference is that **this can actually be enforced.** Trademark law is old, clear and understood
by courts — unlike "evil", which means nothing legally. Mozilla did exactly this with Firefox, and
Debian had to ship "Iceweasel" for years.

The result: anyone may use the code, including someone you have no time for — but **not under your
name**, and without appearing to have your endorsement. And DHS stays packageable everywhere.

Next to it you add a **`VALUES.md`** file, with no legal force, but public and clear: what the
project believes, including its position on age verification. It costs nothing, breaks nothing and
stays on record.

### The three paths, in short

| | A · Apache-2.0 + trademark + `VALUES.md` | B · Apache-2.0 with a custom restriction | C · Hippocratic 3.0 |
|---|---|---|---|
| Is open source | ✅ | ❌ | contested, treated as ❌ |
| Enters Debian/Fedora/Arch | ✅ | ❌ | ❌ |
| Companies can use it without a trip to legal | ✅ | ❌ | ❌ |
| Legally stops "evil" use | ❌ | on paper, hard to enforce | on paper |
| Protects your name and reputation | **✅** | ✅ | ✅ |
| Your position is public and explicit | ✅ via `VALUES.md` | ✅ | ✅ |
| Effort | small | medium + legal risk | small |

**My recommendation: A.** It preserves your goal (runs and gets packaged everywhere), defends your
name with an instrument that actually works, and leaves your position written in black and white.
The restriction in B and C would cost you exactly the distribution in the Linux distributions —
i.e. the audience you are building the tool for — in exchange for a protection that, realistically,
you could not enforce.

If you still choose B, then let us do it **honestly**: a name of its own for the licence, a README
that says it is *source-available*, not open source, and no "Apache" label on it.

## The app database — a separate licence

Data is not code and deserves its own licence, so that anyone can reuse it:

- **CC-BY-4.0** *(recommended)* — anyone can use it, provided they credit you. Matches what you
  asked for.
- **CC0** — public domain, maximum adoption, but you give up the credit.

## What it would mean concretely

```
LICENSE              Apache-2.0, full text
NOTICE               "DHS — Direct Handoff Suite · Copyright 2026 <your name>"
CONTRIBUTING.md      the DCO rule, how to sign, how to add apps to the database
appdb/LICENSE        CC-BY-4.0 for the app database data
short header         in every source file, pointing to LICENSE
```

**Settled 2026-09-03:** the name in `NOTICE` and in the copyright line is the maintainer's public
handle, **Necta** (https://github.com/Necta14). Once it appears in the notices and in the git
history, it is hard to change — so it was chosen deliberately, without a legal entity behind it
for now.

## Sources

- [Cyber Resilience Act — European Commission](https://digital-strategy.ec.europa.eu/en/policies/cyber-resilience-act)
- [The CRA and open source software — OpenSSF](https://openssf.org/public-policy/eu-cyber-resilience-act/)
- [The CRA's obligations for open source — BCLP](https://www.bclplaw.com/en-US/events-insights-news/the-cyber-resilience-acts-obligations-for-open-source-software.html)
- [MIT / Apache / BSD / GPL comparison](https://safeguard.sh/resources/blog/open-source-license-comparison-mit-apache-gpl-bsd)
- [BSD 3-Clause explained — FOSSA](https://fossa.com/blog/open-source-software-licenses-101-bsd-3-clause-license/)
- [A guide to the common licences](https://ghinda.com/opensource/2020/open-source-licenses-apache-mit-bsd.html)
