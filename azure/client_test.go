package azure_test

import (
	"errors"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/cloudfoundry/bosh-bootloader/azure"
	"github.com/cloudfoundry/bosh-bootloader/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client", func() {
	Describe("CheckExists", func() {
		var (
			azureClient *fakes.AzureVNsClient
			client      azure.Client
		)

		BeforeEach(func() {
			azureClient = &fakes.AzureVNsClient{}
			client = azure.NewClientWithInjectedVNsClient(azureClient)
		})

		Context("when the network does not exist", func() {
			BeforeEach(func() {
				otherNetworkName := "some-other-environment-bosh-vn"
				azureClient.ListCall.Returns.Result = network.VirtualNetworkListResult{
					Value: &[]network.VirtualNetwork{
						network.VirtualNetwork{
							Name: &otherNetworkName,
						},
					},
				}
			})
			It("returns false", func() {
				exists, err := client.CheckExists("some-environment")
				Expect(err).NotTo(HaveOccurred())

				Expect(exists).To(BeFalse())
			})
		})

		Context("when the network exists", func() {
			BeforeEach(func() {
				sameNetworkName := "exact-same-bosh-vn"
				azureClient.ListCall.Returns.Result = network.VirtualNetworkListResult{
					Value: &[]network.VirtualNetwork{
						network.VirtualNetwork{
							Name: &sameNetworkName,
						},
					},
				}
			})
			It("returns true", func() {
				exists, err := client.CheckExists("exact-same")
				Expect(err).NotTo(HaveOccurred())

				Expect(exists).To(BeTrue())
			})
		})

		Context("when listing the networks fails", func() {
			BeforeEach(func() {
				azureClient.ListCall.Returns.Error = errors.New("grape")
			})
			It("returns the error", func() {
				_, err := client.CheckExists("some-network")
				Expect(err).To(MatchError("List networks: grape"))
			})
		})
	})

	Describe("ValidateSafeToDelete", func() {
		var (
			azureClient *fakes.AzureVMsClient
			client      azure.Client
		)

		BeforeEach(func() {
			azureClient = &fakes.AzureVMsClient{}
			client = azure.NewClientWithInjectedVMsClient(azureClient)
		})

		Context("when the bosh director and jumpbox are the only vms in the network", func() {
			BeforeEach(func() {
				boshString := "bosh"
				jumpboxString := "jumpbox"

				azureClient.ListCall.Returns.Result = compute.VirtualMachineListResult{
					Value: &[]compute.VirtualMachine{
						compute.VirtualMachine{
							Tags: &map[string]*string{
								"job": &boshString,
							},
						},
						compute.VirtualMachine{
							Tags: &map[string]*string{
								"job": &jumpboxString,
							},
						},
					},
				}
			})

			It("does not return an error ", func() {
				err := client.ValidateSafeToDelete("", "some-env-id")
				Expect(err).NotTo(HaveOccurred())

				Expect(azureClient.ListCall.Receives.ResourceGroup).To(Equal("some-env-id-bosh"))
			})
		})

		Context("when some other bosh deployed vm exists in the network", func() {
			BeforeEach(func() {
				boshString := "bosh"
				jobString := "some-job"
				deploymentString := "some-deployment"
				vmNameString := "some-other-vm"

				azureClient.ListCall.Returns.Result = compute.VirtualMachineListResult{
					Value: &[]compute.VirtualMachine{
						compute.VirtualMachine{
							Tags: &map[string]*string{
								"job": &boshString,
							},
						},
						compute.VirtualMachine{
							Name: &vmNameString,
							Tags: &map[string]*string{
								"job":        &jobString,
								"deployment": &deploymentString,
							},
						},
					},
				}
			})

			It("returns a helpful error message", func() {
				err := client.ValidateSafeToDelete("", "some-env-id")
				Expect(err).To(MatchError(`bbl environment is not safe to delete; vms still exist in resource group: some-env-id-bosh (deployment: some-deployment): some-other-vm`))
			})
		})

		Context("when some other non-bosh deployed vm exists in the network", func() {
			BeforeEach(func() {
				vmNameString := "some-other-vm"
				azureClient.ListCall.Returns.Result = compute.VirtualMachineListResult{
					Value: &[]compute.VirtualMachine{
						compute.VirtualMachine{
							Name: &vmNameString,
							Tags: &map[string]*string{},
						},
					},
				}
			})

			It("returns a helpful error message", func() {
				err := client.ValidateSafeToDelete("", "some-env-id")
				Expect(err).To(MatchError(`bbl environment is not safe to delete; vms still exist in resource group: some-env-id-bosh: some-other-vm`))
			})
		})

		Context("failure cases", func() {
			Context("when azure client list instances fails", func() {
				BeforeEach(func() {
					azureClient.ListCall.Returns.Error = errors.New("passionfruit")
				})

				It("returns an error", func() {
					err := client.ValidateSafeToDelete("some-network", "some-env-id")
					Expect(err).To(MatchError("List instances: passionfruit"))
				})
			})
		})
	})
})
