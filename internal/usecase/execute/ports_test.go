package execute

var (
	_ CommandRunner     = (*fakeRunner)(nil)
	_ Inventory         = (*fakeInventory)(nil)
	_ HomebrewInstaller = (*fakeInstaller)(nil)
	_ StateDir          = (*fakeState)(nil)
	_ Clock             = fakeClock{}
	_ UserInfo          = fakeUser("")
	_ Progress          = (*fakeProgress)(nil)
	_ Confirmer         = fakeConfirmer{}
	_ Replanner         = replanFunc(nil)
)
