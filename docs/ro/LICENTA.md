# Licența — opțiuni și recomandare (D6)

> Traducere în română. Versiunea de referință e cea în engleză: [`../LICENSE-CHOICE.md`](../LICENSE-CHOICE.md)

> Stare: **propunere**, 02.09.2026, scrisă înainte de decizie. **Decis în aceeași zi: drumul A**
> (Apache-2.0 + politică de marcă + `VALUES.md`) — vezi D6 în [`CLAUDE.md`](CLAUDE.md).
> Raționamentul e păstrat integral mai jos.
> Cerințele tale: licență **permisivă**, dar **drepturile tale să rămână recunoscute**, fără
> obligații de tip verificare a vârstei.
>
> ⚠️ Nu sunt avocat; ce urmează e o comparație tehnică, nu consultanță juridică. Pentru o decizie
> cu miză comercială, întreabă pe cineva calificat.

## Întâi, lămurirea despre verificarea vârstei

**Nicio licență open source nu impune verificarea vârstei.** Licențele reglementează cine poate
copia, modifica și redistribui codul — atât. Obligațiile de verificare a vârstei vin din cu totul
altă parte:

- **magazine de aplicații** (Microsoft Store, Google Play, App Store) — au propriile clasificări de
  vârstă, dar acelea sunt chestionare de rating, nu verificare de identitate;
- **servicii online unde utilizatorii interacționează între ei** — forum, chat, conținut încărcat de
  useri. Aici pot intra reglementări ca UK Online Safety Act.

DHS e o unealtă care se descarcă și rulează local, fără conturi, fără server, fără conținut de la
utilizatori. **Nu atinge niciuna dintre categoriile de mai sus.** Dacă vreodată deschizi un forum
sau un Discord oficial, atunci se pune întrebarea — dar pentru serviciul acela, nu pentru licență.

### Ce te privește totuși, ca dezvoltator din UE

**Cyber Resilience Act** (Regulamentul (UE) 2024/2847) — reglementează produsele cu elemente
digitale. Datele care contează:

- **11 septembrie 2026** — intră în vigoare obligațiile de raportare a vulnerabilităților
- **11 decembrie 2027** — restul obligațiilor

Software-ul open source **necomercial** e în mare parte exceptat, iar amenzile nu se aplică
dezvoltatorilor open source necomerciali. Există și o categorie intermediară, „open source steward",
cu obligații mai ușoare. Dacă DHS rămâne gratuit și necomercial, ești aproape sigur în afara zonei
grele. Dacă vreodată vinzi suport, o versiune profesională sau GUI-ul, situația se schimbă și merită
recitit atunci.

## Opțiunile de licență

| | MIT | BSD-3-Clause | **Apache-2.0** | MPL-2.0 |
|---|---|---|---|---|
| Permisivă | ✅ | ✅ | ✅ | parțial |
| Notița de copyright trebuie păstrată | ✅ | ✅ | ✅ | ✅ |
| Mecanism formal de atribuire (fișier `NOTICE`) | ❌ | ❌ | **✅** | ❌ |
| Nu-ți pot folosi numele ca să-și promoveze forkul | ❌ | ✅ | **✅** | ❌ |
| Marca „DHS" rămâne a ta, explicit | ❌ | parțial | **✅** | ❌ |
| Cesiune de brevete de la contribuitori | ❌ | ❌ | **✅** | ✅ |
| Cine te dă în judecată pentru brevete își pierde licența | ❌ | ❌ | **✅** | ✅ |
| Forkul trebuie să spună ce a modificat | ❌ | ❌ | **✅** | ✅ |
| Modificările trebuie să rămână deschise | ❌ | ❌ | ❌ | ✅ (pe fișier) |
| Lungime / frecare | minimă | minimă | medie | medie |

## Recomandare: Apache-2.0

E singura licență permisivă care îți dă, toate odată, exact ce ai cerut:

1. **Atribuire care nu se poate pierde.** Fișierul `NOTICE` trebuie dus mai departe în orice
   derivat. E cel mai solid mecanism de „scrie cine a făcut asta" dintre licențele permisive —
   MIT și BSD se bazează doar pe notița de copyright din antet, pe care lumea o pierde des.
2. **Numele rămâne al tău.** Secțiunea 6 spune explicit că licența **nu** acordă drepturi asupra
   mărcii. Nimeni nu poate scoate un fork și să-i zică „DHS".
3. **Protecție pe brevete.** Contribuitorii îți cedează drepturile de brevet asupra a ce aduc, iar
   cine te dă în judecată pentru brevete își pierde automat licența. MIT și BSD nu spun nimic
   despre brevete — ambiguitatea rămâne.
4. **Forkurile trebuie să declare ce au schimbat.** Deci o versiune stricată de altcineva nu poate
   fi confundată cu a ta.
5. E standardul pe care îl așteaptă contribuitorii serioși și departamentele juridice din firme.

**Prețul:** text mai lung decât MIT și puțină birocrație în plus (`NOTICE`, `LICENSE`, antete). Pentru
o unealtă care umblă la datele oamenilor și care vrea contribuitori, merită.

### Dacă vrei totuși ceva mai simplu

- **MIT** — dacă prioritatea e adopția maximă și zero frecare. Pierzi cesiunea de brevete, clauza de
  marcă și `NOTICE`.
- **BSD-3-Clause** — MIT plus „nu-mi folosi numele ca să-ți promovezi forkul". Bun compromis, dar tot
  fără brevete.

### Ce înseamnă „permisiv", ca să fie alegerea informată

Cu Apache-2.0, cineva **poate** lua DHS, îl poate închide și îl poate vinde. Ce **nu** poate: să
scoată atribuirea, să-i zică „DHS", sau să pretindă că tu îl susții. Dacă asta te deranjează, atunci
răspunsul nu e o licență permisivă, ci **GPLv3** — care obligă orice derivat distribuit să rămână
deschis. Ai spus permisivă, deci merg pe Apache-2.0; îți semnalez doar ce renunți.

## Partea care contează cel mai mult pentru „drepturile mele"

Nu licența, ci **cine deține contribuțiile altora.**

Din clipa în care altcineva trimite cod, **el deține acel cod**. Tu nu mai poți schimba licența
singur și nu mai poți face licențiere duală comercială fără acordul tuturor. Două mecanisme:

| | DCO | CLA |
|---|---|---|
| Ce e | contribuitorul semnează `Signed-off-by`, confirmând că are dreptul să dea codul | contribuitorul îți acordă explicit drepturi largi asupra contribuției |
| Frecare | minimă, o linie în commit | trebuie semnat un document |
| Poți relicenția mai târziu | **nu**, fără acordul fiecăruia | **da** |
| Reacția comunității | bună | unii contribuitori refuză din principiu |

**Retroactiv e aproape imposibil** — ar trebui să prinzi fiecare contribuitor de acum trei ani. Deci
decizia se ia acum, nu mai târziu.

**Propunere:** DCO pentru început. Contribuțiile la un asemenea proiect vin mai ales în baza de
aplicații, adică date, nu cod, iar frecarea mică ajută la creștere. Treci la CLA doar dacă apare
intenția reală de a vinde ceva.

## Apache-2.0 modificat, cu clauză etică — analiza

Cerința: cod liber de folosit oriunde, **cu excepția** scopurilor „diabolice" și a produselor care
fac verificare de vârstă.

### Problema centrală: cele două jumătăți se anulează

Ai spus, în aceeași frază, două lucruri care nu pot coexista:

> „codul este liber de implementat în alte distro-uri/OS-uri" **și** „cât timp nu e folosit în [X]"

Orice restricție de **utilizare** face licența să **nu mai fie open source**, după ambele definiții
care contează în practică:

- **Open Source Definition, criteriul 6** — „fără discriminare pe domenii de activitate": licența nu
  poate interzice folosirea într-un anumit domeniu.
- **Definiția software-ului liber, Libertatea 0** — „să rulezi programul cum vrei, în orice scop".

Iar distribuțiile Linux își iau politicile **direct** din definițiile astea. Consecința e concretă,
nu teoretică: DHS **nu ar mai putea intra** în Debian, Fedora, depozitele oficiale Arch, openSUSE,
nixpkgs sau Homebrew core.

Pentru o unealtă al cărei rost e **să ajute oamenii să treacă pe Linux**, a nu putea fi împachetată
în distribuțiile Linux e aproape fatal. Ai plăti costul ăsta, sigur și imediat, ca să previi un
scenariu aproape ipotetic: cine ar încorpora o unealtă locală de migrare de fișiere într-un produs de
verificare a vârstei?

### Precedentul care s-a jucat deja: licența JSON

Douglas Crockford a adăugat la o licență MIT o singură propoziție: *„The Software shall be used for
Good, not Evil."* Ce a urmat, timp de peste douăzeci de ani:

- clasificată **non-liberă** de Debian, Fedora, Red Hat legal, GNU și FSF;
- **nedistribuibilă** de nicio organizație care garantează libertatea utilizatorilor;
- efect în cascadă — proiecte întregi n-au putut fi împachetate fiindcă *o dependență* avea clauza;
- departamente juridice care au interzis-o din start, fiindcă „evil" nu e definit nicăieri;
- IBM a fost nevoită să ceară oficial permisiunea de a face rău. Crockford a acordat-o, în glumă.

Intenția era bună. Rezultatul a fost două decenii de durere în împachetare și zero rele împiedicate.

### Și problema de nume

Textul licenței Apache e al Apache Software Foundation. Un text modificat **nu mai poate fi numit
„Apache-2.0"** — ar induce în eroare oamenii și uneltele automate (SPDX, scanere de licențe), care
oricum l-ar marca drept licență proprie, necunoscută. Ar trebui botezat altfel, de exemplu
„DHS Community License 1.0", și atunci fiecare firmă care îl vede îl trimite la juridic.

### Că nu ești singur în tabăra asta

Există o mișcare întreagă — *Ethical Source* — și licențe gata făcute, mai ales **Hippocratic
License 3.0**, care interzice folosirea în activități ce încalcă Declarația Universală a Drepturilor
Omului. Autorii ei susțin că respectă definiția open source, fiindcă restricțiile țintesc *activități*,
nu *categorii de oameni*. OSI nu a aprobat-o, iar distribuțiile o tratează ca non-liberă. Deci
poziția ta are companie și argumente; are însă și același cost practic.

### Ce funcționează de fapt: marca, nu licența

Nu poți controla legal **utilizarea** codului fără să pierzi statutul de open source. Dar poți
controla foarte bine **numele tău** — iar numele e cel care poartă reputația.

Apache-2.0 §6 îți rezervă deja marca. Peste asta publici o **politică de marcă**:

> Codul e liber, sub Apache-2.0. Numele „DHS", „Direct Handoff Suite" și sigla sunt însă mărcile
> mele. Le poți folosi doar pentru versiuni nemodificate. Dacă încorporezi acest cod într-un produs
> care implementează verificarea vârstei, nu ai dreptul să-l numești DHS, să folosești sigla sau să
> spui „powered by DHS". Redenumește-l.

Diferența e că **asta chiar se poate aplica.** Dreptul mărcilor e vechi, clar și înțeles de instanțe
— spre deosebire de „diabolic", care nu înseamnă nimic juridic. Mozilla a făcut exact asta cu
Firefox, iar Debian a fost nevoită ani de zile să livreze „Iceweasel".

Rezultatul: oricine poate folosi codul, inclusiv cineva de care n-ai chef — dar **nu sub numele
tău**, și fără să pară că-l susții. Și DHS rămâne împachetabil peste tot.

Adaugi lângă asta un fișier **`VALUES.md`**, fără forță juridică, dar public și clar: ce crede
proiectul, inclusiv poziția față de verificarea vârstei. Nu costă nimic, nu strică nimic și rămâne
scris.

### Cele trei drumuri, pe scurt

| | A · Apache-2.0 + marcă + `VALUES.md` | B · Apache-2.0 cu restricție proprie | C · Hippocratic 3.0 |
|---|---|---|---|
| E open source | ✅ | ❌ | contestat, tratat ca ❌ |
| Intră în Debian/Fedora/Arch | ✅ | ❌ | ❌ |
| Firmele îl pot folosi fără drum la juridic | ✅ | ❌ | ❌ |
| Oprește legal folosirea „diabolică" | ❌ | pe hârtie, greu de aplicat | pe hârtie |
| Îți protejează numele și reputația | **✅** | ✅ | ✅ |
| Poziția ta e publică și explicită | ✅ prin `VALUES.md` | ✅ | ✅ |
| Efort | mic | mediu + risc juridic | mic |

**Recomandarea mea: A.** Îți păstrează scopul (rulează și se împachetează oriunde), îți apără numele
cu un instrument care chiar funcționează, și îți lasă poziția scrisă negru pe alb. Restricția din B
și C ți-ar costa exact distribuția în distribuțiile Linux — adică publicul pentru care construiești
unealta — în schimbul unei protecții pe care, realist, n-ai putea-o pune în aplicare.

Dacă alegi totuși B, atunci hai s-o facem **cinstit**: nume propriu al licenței, scris în README că
e *source-available*, nu open source, și fără eticheta „Apache" pe ea.

## Baza de date de aplicații — licență separată

Datele nu sunt cod și merită licența lor, ca să poată fi refolosite de oricine:

- **CC-BY-4.0** *(recomandat)* — oricine o poate folosi, cu condiția să te crediteze. Se potrivește
  cu ce ai cerut.
- **CC0** — domeniu public, adopție maximă, dar renunți la credit.

## Ce ar însemna concret

```
LICENSE              Apache-2.0, text integral
NOTICE               "DHS — Direct Handoff Suite · Copyright 2026 <numele tău>"
CONTRIBUTING.md      regula DCO, cum se semnează, cum se adaugă aplicații în bază
appdb/LICENSE        CC-BY-4.0 pentru datele din baza de aplicații
antet scurt          în fiecare fișier sursă, cu trimitere la LICENSE
```

**Stabilit 03.09.2026:** numele din `NOTICE` și din linia de copyright e pseudonimul public al
menținătorului, **Necta** (https://github.com/Necta14). Odată ce apare în notițe și în istoricul
git, se schimbă greu — de aceea a fost ales deliberat, fără o entitate juridică în spate, deocamdată.

## Surse

- [Cyber Resilience Act — Comisia Europeană](https://digital-strategy.ec.europa.eu/en/policies/cyber-resilience-act)
- [CRA și software-ul open source — OpenSSF](https://openssf.org/public-policy/eu-cyber-resilience-act/)
- [Obligațiile CRA pentru open source — BCLP](https://www.bclplaw.com/en-US/events-insights-news/the-cyber-resilience-acts-obligations-for-open-source-software.html)
- [Comparație MIT / Apache / BSD / GPL](https://safeguard.sh/resources/blog/open-source-license-comparison-mit-apache-gpl-bsd)
- [BSD 3-Clause explicată — FOSSA](https://fossa.com/blog/open-source-software-licenses-101-bsd-3-clause-license/)
- [Ghid pe licențele uzuale](https://ghinda.com/opensource/2020/open-source-licenses-apache-mit-bsd.html)
