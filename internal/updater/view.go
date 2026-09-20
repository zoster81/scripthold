package updater

import "context"

// SelfUpdateView composes local installation evidence with the legacy remote
// release check without allowing remote availability to alter local recovery.
type SelfUpdateView struct {
	Installation             InstallationStatus
	ReleaseComparisonVersion string
	Release                  ReleaseCheckResult
}

type selfUpdateViewDeps struct {
	observeInstallation func(context.Context) (InstallationStatus, error)
	checkRelease        func(context.Context, string, bool) ReleaseCheckResult
}

// ObserveSelfUpdateView combines local installation status with the existing
// cached release check. Remote comparison is skipped when an adopted target
// does not have a fully verified installed semantic version.
func ObserveSelfUpdateView(
	ctx context.Context,
	processVersion string,
	force bool,
) (SelfUpdateView, error) {
	return observeSelfUpdateViewWith(ctx, processVersion, force, selfUpdateViewDeps{
		observeInstallation: ObserveCurrentInstallationStatus,
		checkRelease:        CheckRelease,
	})
}

func observeSelfUpdateViewWith(
	ctx context.Context,
	processVersion string,
	force bool,
	deps selfUpdateViewDeps,
) (SelfUpdateView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if deps.observeInstallation == nil {
		deps.observeInstallation = ObserveCurrentInstallationStatus
	}
	if deps.checkRelease == nil {
		deps.checkRelease = CheckRelease
	}

	installation, err := deps.observeInstallation(ctx)
	if err != nil {
		return SelfUpdateView{}, err
	}
	view := SelfUpdateView{Installation: installation}

	comparisonVersion := processVersion
	if installation.Adopted {
		comparisonVersion = installation.InstalledVersion
	}
	if comparisonVersion == "" {
		return view, nil
	}

	view.ReleaseComparisonVersion = comparisonVersion
	view.Release = deps.checkRelease(ctx, comparisonVersion, force)
	return view, nil
}
