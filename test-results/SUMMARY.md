# Results · symmetrical-space-capybara-v6g4x96qg5v6hjj9 · 2026-09-04T10:51:00Z

- commit: cfd87a2 · appdb test: GitHub Desktop is in the database now; use a name that is not
- go: go1.26.1
- machine: 4 cores · 15Gi RAM · Linux 6.8.0-1052-azure

| step | result |
|---|---|
| gofmt | ✅ 0 s |
| vet | ✅ 0 s |
| vet-windows | ✅ 0 s |
| build-linux | ✅ 0 s |
| build-windows | ✅ 1 s |
| build-arm64 | ✅ 0 s |
| build-gui-win | ✅ 1 s |
| build-gui-arm | ✅ 0 s |
| gui-linux-syntax | ✅ 0 s |
| test | ✅ 2 s |
| race | ✅ 10 s |
| cli-version | ✅ 0 s |
| e2e | ✅ 27 s |

## Binaries
- /tmp/dhs: 5.3 MiB
- /tmp/dhs.exe: 5.6 MiB

## Tests (excerpt)
```
--- PASS: TestEmbeddedDatabaseValidates (0.03s)
--- PASS: TestQueries (0.00s)
--- PASS: TestExcluded (0.00s)
--- PASS: TestValidationCatchesMistakes (0.00s)
PASS
ok  	github.com/Necta14/dhs/appdb	0.039s
--- PASS: TestResolve (0.00s)
--- PASS: TestPacmanDesc (0.00s)
--- PASS: TestDpkgStatus (0.00s)
--- PASS: TestApkInstalled (0.00s)
--- PASS: TestDesktopEntries (0.00s)
--- PASS: TestSnapYAMLVersion (0.00s)
--- PASS: TestDetectorMatchesAndFindsConfig (0.00s)
--- PASS: TestRegistryAndScoopMatching (0.00s)
--- PASS: TestPlanWindowsToArch (0.00s)
--- PASS: TestPlanPrefersFlatpakLocationWhenInstalledThatWay (0.00s)
--- PASS: TestPlanMissingAndNotInDatabase (0.00s)
--- PASS: TestRunnerRetriesBatchOneByOne (0.00s)
--- PASS: TestSnapAndWingetCommandsAreOnePerPackage (0.00s)
--- PASS: TestAppRootRoundTrip (0.00s)
PASS
ok  	github.com/Necta14/dhs/internal/apps	0.010s
--- PASS: TestRoundTripEncrypted (0.67s)
--- PASS: TestRoundTripUnencrypted (0.06s)
--- PASS: TestManifestLeaksNothing (0.12s)
--- PASS: TestPassphraseErrors (0.04s)
--- PASS: TestDedup (0.13s)
--- PASS: TestLargeFileSpansVolumes (0.14s)
--- PASS: TestFilterSkipsUnneededVolumes (0.12s)
--- PASS: TestCorruptionIsDetected (0.06s)
--- PASS: TestAbortLeavesIncompletePackage (0.01s)
--- PASS: TestRefusesNonEmptyDir (0.00s)
--- PASS: TestDuplicateEntryRefused (0.00s)
--- PASS: TestSizeMismatchIsRecorded (0.02s)
--- PASS: TestDuplicateInSolidBlockKeepsItsParts (0.05s)
--- PASS: TestRootMapSplitJoin (0.00s)
--- SKIP: TestOtherKeyWindows (0.00s)
PASS
ok  	github.com/Necta14/dhs/internal/pack	1.416s
--- PASS: TestSanitizeWindows (0.00s)
--- PASS: TestSanitizeLinuxTouchesAlmostNothing (0.00s)
--- PASS: TestWithSuffix (0.00s)
--- PASS: TestFoldKey (0.00s)
--- PASS: TestPlanAndExecute (0.04s)
--- PASS: TestPlanConflictPolicies (0.02s)
--- PASS: TestPlanCaseCollisionOnWindows (0.01s)
--- PASS: TestPlanFilter (0.01s)
--- PASS: TestParseConflict (0.00s)
PASS
ok  	github.com/Necta14/dhs/internal/restore	0.083s
--- PASS: TestClassOf (0.00s)
--- PASS: TestClassOfDoubleExtension (0.00s)
--- PASS: TestClassCompressible (0.00s)
--- PASS: TestIsSecret (0.00s)
--- PASS: TestExcluder (0.00s)
--- PASS: TestEstimateIncompressibleStaysWhole (0.00s)
--- PASS: TestEstimateOrderingAcrossLevels (0.00s)
--- PASS: TestEstimateVolumes (0.00s)
--- PASS: TestEstimateSamplingOverridesTable (0.00s)
--- PASS: TestEstimateWorkersReduceTime (0.00s)
--- PASS: TestEstimateEmpty (0.00s)
--- PASS: TestLevelValid (0.00s)
--- PASS: TestWalkClassifiesAndExcludes (0.01s)
--- PASS: TestWalkIncludeSecrets (0.01s)
--- PASS: TestWalkLargestAndCallbacks (0.01s)
--- PASS: TestWalkAllowExclusion (0.00s)
--- PASS: TestWalkSkipsSymlinks (0.00s)
--- PASS: TestWalkSingleFileRoot (0.00s)
--- PASS: TestWalkNoRoots (0.00s)
--- PASS: TestHomeRootsDropsNestedAndMissing (0.00s)
PASS
ok  	github.com/Necta14/dhs/internal/scan	0.037s
--- PASS: TestDetect (0.00s)
--- PASS: TestLocationsCoversEveryKind (0.00s)
--- PASS: TestSpaceOf (0.00s)
--- PASS: TestFAT32FileLimit (0.00s)
PASS
ok  	github.com/Necta14/dhs/internal/system	0.004s
```
