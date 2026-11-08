package generate

import (
	"context"

	"konjiyami/client/console"
	"konjiyami/protobuf/clientpb"
	"konjiyami/protobuf/commonpb"
)

// GetKONJIYAMIBinary - Get the binary of an implant based on it's profile
func GetKONJIYAMIBinary(profile *clientpb.ImplantProfile, con *console.KonjiyamiConsoleClient) ([]byte, error) {
	var data []byte
	// get implant builds
	builds, err := con.Rpc.ImplantBuilds(context.Background(), &commonpb.Empty{})
	if err != nil {
		return data, err
	}

	implantName := buildImplantName(profile.GetConfig().GetFileName())
	_, ok := builds.GetConfigs()[implantName]
	if implantName == "" || !ok {
		// no built implant found for profile, generate a new one
		con.PrintInfof("No builds found for profile %s, generating a new one\n", profile.GetName())
		ctrl := make(chan bool)
		con.SpinUntil("Compiling, please wait ...", ctrl)

		generated, err := con.Rpc.Generate(context.Background(), &clientpb.GenerateReq{
			Config: profile.Config,
		})
		ctrl <- true
		<-ctrl
		if err != nil {
			con.PrintErrorf("Error generating implant\n")
			return data, err
		}
		data = generated.GetFile().GetData()
		profile.Config.FileName = generated.File.Name
		_, err = con.Rpc.SaveImplantProfile(context.Background(), profile)
		if err != nil {
			con.PrintErrorf("Error updating implant profile\n")
			return data, err
		}
		con.PrintInfof("KonjiYami name for profile %s: %s\n", profile.Name, buildImplantName(profile.GetConfig().GetFileName()))
	} else {
		// Found a build, reuse that one
		con.PrintInfof("KonjiYami name for profile %s: %s\n", profile.Name, implantName)
		regenerate, err := con.Rpc.Regenerate(context.Background(), &clientpb.RegenerateReq{
			ImplantName: implantName,
		})

		if err != nil {
			return data, err
		}
		data = regenerate.GetFile().GetData()
	}
	return data, err
}

