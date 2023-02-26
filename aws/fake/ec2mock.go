package fake

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)

const dipSplitSize = 2

type Ec2MockOutputs struct {
	DescribeSecurityGroups *ApiResponse
	DescribeInstances      *ApiResponse
	DescribeInstancesPages []*ApiResponse
	DescribeSubnets        *ApiResponse
	DescribeRouteTables    *ApiResponse
}

type MockEc2Client struct {
	ec2iface.EC2API
	Outputs Ec2MockOutputs
}

func (m *MockEc2Client) DescribeSecurityGroups(*ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
	if out, ok := m.Outputs.DescribeSecurityGroups.response.(*ec2.DescribeSecurityGroupsOutput); ok {
		return out, m.Outputs.DescribeSecurityGroups.err
	}
	return nil, m.Outputs.DescribeSecurityGroups.err
}

func (m *MockEc2Client) DescribeInstances(*ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	if out, ok := m.Outputs.DescribeInstances.response.(*ec2.DescribeInstancesOutput); ok {
		return out, m.Outputs.DescribeInstances.err
	}
	return nil, m.Outputs.DescribeInstances.err
}

func (m *MockEc2Client) DescribeInstancesPages(params *ec2.DescribeInstancesInput, f func(*ec2.DescribeInstancesOutput, bool) bool) error {
	for _, resp := range m.Outputs.DescribeInstancesPages {
		if out, ok := resp.response.(*ec2.DescribeInstancesOutput); ok {
			f(out, true)
		}
	}
	if len(m.Outputs.DescribeInstancesPages) != 0 {
		return m.Outputs.DescribeInstancesPages[0].err
	}
	return nil
}

func (m *MockEc2Client) DescribeSubnets(*ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	if out, ok := m.Outputs.DescribeSubnets.response.(*ec2.DescribeSubnetsOutput); ok {
		return out, m.Outputs.DescribeSubnets.err
	}
	return nil, m.Outputs.DescribeSubnets.err
}

func (m *MockEc2Client) DescribeRouteTables(*ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
	if out, ok := m.Outputs.DescribeRouteTables.response.(*ec2.DescribeRouteTablesOutput); ok {
		return out, m.Outputs.DescribeRouteTables.err
	}
	return nil, m.Outputs.DescribeRouteTables.err
}

func MockDSGOutput(sgs map[string]string) *ec2.DescribeSecurityGroupsOutput {
	groups := make([]*ec2.SecurityGroup, 0)
	for id, name := range sgs {
		sg := &ec2.SecurityGroup{
			GroupId:   aws.String(id),
			GroupName: aws.String(name),
		}
		groups = append(groups, sg)
	}
	return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: groups}
}

type TestInstance struct {
	Id        string
	Tags      Tags
	PrivateIp string
	VpcId     string
	State     int64
}

func MockDIOutput(mockedInstances ...TestInstance) *ec2.DescribeInstancesOutput {
	instances := make([]*ec2.Instance, 0, len(mockedInstances))
	for _, i := range mockedInstances {
		tags := make([]*ec2.Tag, 0, len(i.Tags))
		for k, v := range i.Tags {
			tags = append(tags, &ec2.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		instance := &ec2.Instance{
			InstanceId:       aws.String(i.Id),
			Tags:             tags,
			State:            &ec2.InstanceState{Code: aws.Int64(i.State)},
			PrivateIpAddress: aws.String(i.PrivateIp),
			VpcId:            aws.String(i.VpcId),
		}
		instances = append(instances, instance)
	}
	return &ec2.DescribeInstancesOutput{Reservations: []*ec2.Reservation{{Instances: instances}}}
}

func MockDIPOutput(e error, mockedInstances ...TestInstance) []*ApiResponse {
	pages := len(mockedInstances) / dipSplitSize
	result := make([]*ApiResponse, pages, pages+1)
	for i := 0; i < pages; i++ {
		result[i] = R(MockDIOutput(mockedInstances[i*dipSplitSize:(i+1)*dipSplitSize]...), e)
	}
	if len(mockedInstances)%dipSplitSize != 0 {
		result = append(result, R(MockDIOutput(mockedInstances[pages*dipSplitSize:]...), e))
	}
	return result
}

type TestSubnet struct {
	Id   string
	Az   string
	Name string
	Tags map[string]string
}

func MockDSOutput(mockedSubnets ...TestSubnet) *ec2.DescribeSubnetsOutput {
	subnets := make([]*ec2.Subnet, 0, len(mockedSubnets))
	for _, subnet := range mockedSubnets {
		s := &ec2.Subnet{
			SubnetId:         aws.String(subnet.Id),
			AvailabilityZone: aws.String(subnet.Az),
			Tags: []*ec2.Tag{
				{Key: aws.String("Name"), Value: aws.String(subnet.Name)},
			},
		}
		for k, v := range subnet.Tags {
			s.Tags = append(s.Tags, &ec2.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		subnets = append(subnets, s)
	}
	return &ec2.DescribeSubnetsOutput{Subnets: subnets}
}

type TestRouteTable struct {
	SubnetID   string
	Main       bool
	GatewayIds []string
}

func MockDRTOutput(mockedRouteTables ...TestRouteTable) *ec2.DescribeRouteTablesOutput {
	routeTables := make([]*ec2.RouteTable, 0, len(mockedRouteTables))
	for _, mrt := range mockedRouteTables {
		routes := make([]*ec2.Route, 0, len(mrt.GatewayIds))
		for _, gwID := range mrt.GatewayIds {
			routes = append(routes, &ec2.Route{GatewayId: aws.String(gwID)})
		}
		rt := &ec2.RouteTable{
			Associations: []*ec2.RouteTableAssociation{
				{SubnetId: aws.String(mrt.SubnetID), Main: aws.Bool(mrt.Main)},
			},
			Routes: routes,
		}
		routeTables = append(routeTables, rt)
	}
	return &ec2.DescribeRouteTablesOutput{RouteTables: routeTables}
}
