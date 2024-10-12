package bcel

import "testing"

func Test_preprocessExpressions(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"Simple map access", ".role_name", "cols['role_name']"},
		{"Map access with comparison", ".role_name == 'Admin'", "cols['role_name'] == 'Admin'"},
		{"Multiple map accesses", ".role_name == .another_field", "cols['role_name'] == cols['another_field']"},
		{"Object field access", "user.role_name", "user.role_name"},
		{"Mixed object and map access", "user.role_name == .role_name", "user.role_name == cols['role_name']"},
		{"Function call with map access", "user.get_role(.role_name)", "user.get_role(cols['role_name'])"},
		{"Function call on the left, map access on the right", "someFunc() == .role_name", "someFunc() == cols['role_name']"},
		{"Logical expression with object and map access", "user.name == \"John\" && .is_admin == true", "user.name == \"John\" && cols['is_admin'] == true"},
		{"Array/map access with mixed dot", "person['age'] > 30 && .age == 25", "person['age'] > 30 && cols['age'] == 25"},
		{"String concatenation with map access", ".role_name + \"User\"", "cols['role_name'] + \"User\""},
		{"String concatenation with text and map access", "\"The role is: \" + .role_name", "\"The role is: \" + cols['role_name']"},
		{"Math operation with map and object fields", "10 * .salary + user.bonus", "10 * cols['salary'] + user.bonus"},
		{"Null comparison with map access", ".role_name == null", "cols['role_name'] == null"},
		{"Empty string comparison with map access", ".role_name == \"\"", "cols['role_name'] == \"\""},
		{"Function call on object with map access", "myObject.doSomething(.role_name)", "myObject.doSomething(cols['role_name'])"},
		{"Simple bare string", "alert", "\"alert\""},
		{"Bare string with numeric identifier", "status123", "\"status123\""},
		{"Simple column replacement", ".role_name", "cols['role_name']"},
		{"Bare string with special characters", "status_code", "\"status_code\""},
		{"Mixed column replacement and string", ".role_name == alert", "cols['role_name'] == alert"},
		{"Complex expression with column and bare string", "user.role == .role_name && state == ok", "user.role == cols['role_name'] && state == ok"},
		{"Quoted string in expression", "user.role == 'admin'", "user.role == 'admin'"},
		{"Function call with column access", "check_role(.role_name)", "check_role(cols['role_name'])"},
		{"Bare string with existing quotes", "'alert'", "'alert'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := preprocessExpressions(tt.expr); got != tt.want {
				t.Errorf("preprocessExpressions() = %v, want %v", got, tt.want)
			}
		})
	}
}
