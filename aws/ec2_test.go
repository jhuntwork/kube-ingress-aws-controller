package aws

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/zalando-incubator/kube-ingress-aws-controller/aws/fake"

)

func TestGetAutoScalingName(t *testing.T) {
	for _, test := range []struct {
		tags      map[string]string
		want      string
		wantError bool
	}{
		{map[string]string{autoScalingGroupNameTag: "foo"}, "foo", false},
		{map[string]string{autoScalingGroupNameTag: "foo", "bar": "baz", "zbr": "42"}, "foo", false},
		{map[string]string{"foo": "bar"}, "", true},
		{nil, "", true},
	} {
		t.Run(fmt.Sprintf("want-%s", test.want), func(t *testing.T) {
			got, err := getAutoScalingGroupName(test.tags)
			assertResultAndError(t, test.want, got, test.wantError, err)
		})
	}

}

func TestFindingSecurityGroup(t *testing.T) {
	for _, test := range []struct {
		name      string
		responses fake.Ec2MockOutputs
		want      *securityGroupDetails
		wantError bool
	}{
		{
			"success-find-sg",
			fake.Ec2MockOutputs{
				DescribeSecurityGroups: fake.R(fake.MockDSGOutput(map[string]string{"id": "foo"}), nil),
			},
			&securityGroupDetails{id: "id", name: "foo"},
			false,
		},
		{
			"fail-no-security-groups",
			fake.Ec2MockOutputs{DescribeSecurityGroups: fake.R(fake.MockDSGOutput(nil), nil)}, nil, true,
		},
		{
			"fail-with-aws-api-error",
			fake.Ec2MockOutputs{DescribeSecurityGroups: fake.R(nil, fake.ErrDummy)}, nil, true,
		},
	} {
		t.Run(fmt.Sprintf("%v", test.name), func(t *testing.T) {
			ec2 := &fake.MockEc2Client{Outputs: test.responses}
			got, err := findSecurityGroupWithClusterID(ec2, "foo", "kube-ingress-aws-controller")
			assertResultAndError(t, test.want, got, test.wantError, err)
		})
	}
}

func TestInstanceDetails(t *testing.T) {
	for _, test := range []struct {
		given         instanceDetails
		wantClusterID string
	}{
		{
			given: instanceDetails{id: "this-should-be-fine", vpcID: "bar", tags: map[string]string{
				nameTag:                    "baz",
				clusterIDTagPrefix + "zbr": resourceLifecycleOwned,
			}},
			wantClusterID: "zbr",
		},
		{
			given: instanceDetails{id: "this-should-be-fine-legacy", vpcID: "bar", tags: map[string]string{
				nameTag:                    "baz",
				kubernetesClusterLegacyTag: "zbr",
			}},
			wantClusterID: "zbr",
		},
		{
			given: instanceDetails{id: "this-should-be-fine-new-plus-legacy", vpcID: "bar", tags: map[string]string{
				nameTag:                    "foo",
				kubernetesClusterLegacyTag: "bar",
				clusterIDTagPrefix + "baz": resourceLifecycleOwned,
			}},
			wantClusterID: "baz",
		},
		{
			given: instanceDetails{id: "missing-name-tag", vpcID: "bar", tags: map[string]string{
				clusterIDTagPrefix + "zbr": resourceLifecycleOwned,
			}},
			wantClusterID: "zbr",
		},
		{
			given: instanceDetails{id: "missing-cluster-id-tag", vpcID: "bar", tags: map[string]string{
				nameTag: "baz",
			}},
			wantClusterID: defaultClusterID,
		},
		{
			given:         instanceDetails{id: "missing-mgmt-tags", vpcID: "bar", tags: map[string]string{}},
			wantClusterID: defaultClusterID,
		},
		{
			given:         instanceDetails{id: "nil-mgmt-tags", vpcID: "bar"},
			wantClusterID: defaultClusterID,
		},
	} {
		t.Run(fmt.Sprintf("%v", test.given.id), func(t *testing.T) {
			if test.given.clusterID() != test.wantClusterID {
				t.Errorf("unexpected cluster ID. wanted %q, got %q", test.wantClusterID, test.given.clusterID())
			}
		})
	}
}

func TestGetInstanceDetails(t *testing.T) {
	for _, test := range []struct {
		name      string
		responses fake.Ec2MockOutputs
		want      *instanceDetails
		wantError bool
	}{
		{
			"success-call",
			fake.Ec2MockOutputs{DescribeInstances: fake.R(fake.MockDIOutput(
				fake.TestInstance{Id: "foo", Tags: fake.Tags{"bar": "baz"}, State: runningState},
			), nil)},
			&instanceDetails{id: "foo", tags: map[string]string{"bar": "baz"}, running: true},
			false,
		},
		{
			"failed-state-match",
			fake.Ec2MockOutputs{DescribeInstances: fake.R(fake.MockDIOutput(
				fake.TestInstance{Id: "foo1", Tags: fake.Tags{"bar": "baz"}, State: 0},
				fake.TestInstance{Id: "foo2", Tags: fake.Tags{"bar": "baz"}, State: 32}, // shutting-down
				fake.TestInstance{Id: "foo2", Tags: fake.Tags{"bar": "baz"}, State: 48}, // terminated includes running?!?
				fake.TestInstance{Id: "foo2", Tags: fake.Tags{"bar": "baz"}, State: 64}, // stopping
				fake.TestInstance{Id: "foo2", Tags: fake.Tags{"bar": "baz"}, State: 80}, // stopped includes running?!?
			), nil)},
			nil,
			true,
		},
		{
			"nothing-returned-from-aws-api",
			fake.Ec2MockOutputs{DescribeInstances: fake.R(fake.MockDIOutput(), nil)},
			nil,
			true,
		},
		{
			"aws-api-fail",
			fake.Ec2MockOutputs{DescribeInstances: fake.R(nil, fake.ErrDummy)},
			nil,
			true,
		},
	} {
		t.Run(fmt.Sprintf("%v", test.name), func(t *testing.T) {
			ec2 := &fake.MockEc2Client{Outputs: test.responses}
			got, err := getInstanceDetails(ec2, "foo")
			assertResultAndError(t, test.want, got, test.wantError, err)
		})
	}
}

func TestGetSubnets(t *testing.T) {
	for _, test := range []struct {
		name      string
		responses fake.Ec2MockOutputs
		want      []*subnetDetails
		wantError bool
	}{
		{
			"success-call-nofilter",
			fake.Ec2MockOutputs{
				DescribeSubnets: fake.R(fake.MockDSOutput(
					fake.TestSubnet{Id: "foo1", Name: "bar1", Az: "baz1", Tags: map[string]string{elbRoleTagName: ""}},
					fake.TestSubnet{Id: "foo2", Name: "bar2", Az: "baz2"},
				), nil),
				DescribeRouteTables: fake.R(fake.MockDRTOutput(
					fake.TestRouteTable{SubnetID: "foo1", GatewayIds: []string{"igw-foo1"}},
					fake.TestRouteTable{SubnetID: "mismatch", GatewayIds: []string{"igw-foo2"}, Main: true},
				), nil),
			},
			[]*subnetDetails{
				{id: "foo1", availabilityZone: "baz1", public: true, tags: map[string]string{nameTag: "bar1", elbRoleTagName: ""}},
				{id: "foo2", availabilityZone: "baz2", public: true, tags: map[string]string{nameTag: "bar2"}},
			},
			false,
		},
		{
			"success-call-filtered",
			fake.Ec2MockOutputs{
				DescribeSubnets: fake.R(fake.MockDSOutput(
					fake.TestSubnet{Id: "foo1", Name: "bar1", Az: "baz1", Tags: map[string]string{elbRoleTagName: "", clusterIDTagPrefix + "bar": "shared"}},
					fake.TestSubnet{Id: "foo2", Name: "bar2", Az: "baz2", Tags: map[string]string{clusterIDTagPrefix + "bar": "shared"}},
				), nil),
				DescribeRouteTables: fake.R(fake.MockDRTOutput(
					fake.TestRouteTable{SubnetID: "foo1", GatewayIds: []string{"igw-foo1"}},
					fake.TestRouteTable{SubnetID: "mismatch", GatewayIds: []string{"igw-foo2"}, Main: true},
				), nil),
			},
			[]*subnetDetails{
				{id: "foo1", availabilityZone: "baz1", public: true, tags: map[string]string{nameTag: "bar1", elbRoleTagName: "", clusterIDTagPrefix + "bar": "shared"}},
				{id: "foo2", availabilityZone: "baz2", public: true, tags: map[string]string{nameTag: "bar2", clusterIDTagPrefix + "bar": "shared"}},
			},
			false,
		},
		{
			"aws-sdk-failure-describing-subnets",
			fake.Ec2MockOutputs{DescribeSubnets: fake.R(nil, fake.ErrDummy)}, nil, true,
		},
		{
			"aws-sdk-failure-describing-route-tables",
			fake.Ec2MockOutputs{
				DescribeSubnets: fake.R(fake.MockDSOutput(
					fake.TestSubnet{Id: "foo1", Name: "bar1", Az: "baz1"},
					fake.TestSubnet{Id: "foo2", Name: "bar2", Az: "baz2"},
				), nil),
				DescribeRouteTables: fake.R(nil, fake.ErrDummy),
			}, nil, true,
		},
		{
			"failure-to-map-subnets",
			fake.Ec2MockOutputs{
				DescribeSubnets: fake.R(fake.MockDSOutput(
					fake.TestSubnet{Id: "foo1", Name: "bar1", Az: "baz1"},
				), nil),
				DescribeRouteTables: fake.R(fake.MockDRTOutput(
					fake.TestRouteTable{SubnetID: "x", GatewayIds: []string{"y"}},
				), nil),
			},
			nil, true,
		},
	} {
		t.Run(fmt.Sprintf("%v", test.name), func(t *testing.T) {
			ec2 := &fake.MockEc2Client{Outputs: test.responses}
			got, err := getSubnets(ec2, "foo", "bar")
			assertResultAndError(t, test.want, got, test.wantError, err)
		})
	}
}

func TestGetInstancesDetailsWithFilters(t *testing.T) {
	for _, test := range []struct {
		name      string
		input     []*ec2.Filter
		responses fake.Ec2MockOutputs
		want      map[string]*instanceDetails
		wantError bool
	}{
		{
			"success-call",
			[]*ec2.Filter{
				{
					Name: aws.String("tag:KubernetesCluster"),
					Values: []*string{
						aws.String("kube1"),
					},
				},
			},
			fake.Ec2MockOutputs{DescribeInstancesPages: fake.MockDIPOutput(
				nil,
				fake.TestInstance{Id: "foo1", Tags: fake.Tags{"bar": "baz"}, PrivateIp: "1.2.3.4", VpcId: "1", State: 16},
				fake.TestInstance{Id: "foo2", Tags: fake.Tags{"bar": "baz"}, PrivateIp: "1.2.3.5", VpcId: "1", State: 32},
				fake.TestInstance{Id: "foo3", Tags: fake.Tags{"aaa": "zzz"}, PrivateIp: "1.2.3.6", VpcId: "1", State: 80},
			)},
			map[string]*instanceDetails{
				"foo1": &instanceDetails{id: "foo1", tags: map[string]string{"bar": "baz"}, ip: "1.2.3.4", vpcID: "1", running: true},
				"foo2": &instanceDetails{id: "foo2", tags: map[string]string{"bar": "baz"}, ip: "1.2.3.5", vpcID: "1", running: false},
				"foo3": &instanceDetails{id: "foo3", tags: map[string]string{"aaa": "zzz"}, ip: "1.2.3.6", vpcID: "1", running: false},
			},
			false,
		},
		{
			"success-empty-filters",
			[]*ec2.Filter{},
			fake.Ec2MockOutputs{DescribeInstancesPages: fake.MockDIPOutput(
				nil,
				fake.TestInstance{Id: "foo1", Tags: fake.Tags{"bar": "baz"}, PrivateIp: "1.2.3.4", VpcId: "1", State: 16},
				fake.TestInstance{Id: "foo3", Tags: fake.Tags{"aaa": "zzz"}, PrivateIp: "1.2.3.6", VpcId: "1", State: 80},
			)},
			map[string]*instanceDetails{
				"foo1": &instanceDetails{id: "foo1", tags: map[string]string{"bar": "baz"}, ip: "1.2.3.4", vpcID: "1", running: true},
				"foo3": &instanceDetails{id: "foo3", tags: map[string]string{"aaa": "zzz"}, ip: "1.2.3.6", vpcID: "1", running: false},
			},
			false,
		},
		{
			"success-empty-response",
			[]*ec2.Filter{
				{
					Name: aws.String("vpc-id"),
					Values: []*string{
						aws.String("some-vpc"),
					},
				},
			},
			fake.Ec2MockOutputs{DescribeInstancesPages: fake.MockDIPOutput(nil)},
			map[string]*instanceDetails{},
			false,
		},
		{
			"aws-api-fail",
			[]*ec2.Filter{
				{
					Name: aws.String("tag-key"),
					Values: []*string{
						aws.String("key1"),
					},
				},
			},
			fake.Ec2MockOutputs{DescribeInstancesPages: fake.MockDIPOutput(fake.ErrDummy, fake.TestInstance{})},
			nil,
			true,
		},
	} {
		t.Run(fmt.Sprintf("%v", test.name), func(t *testing.T) {
			ec2 := &fake.MockEc2Client{Outputs: test.responses}
			got, err := getInstancesDetailsWithFilters(ec2, test.input)
			assertResultAndError(t, test.want, got, test.wantError, err)
		})
	}
}

func assertResultAndError(t *testing.T, want, got interface{}, wantError bool, err error) {
	if wantError {
		if err == nil {
			t.Error("wanted an error but call seemed to have succeeded")
		}
	} else {
		if err != nil {
			t.Fatal("unexpected error", err)
		}

		if !reflect.DeepEqual(want, got) {
			t.Errorf("unexpected result. wanted %+v, got %+v", want, got)
		}
	}
}
