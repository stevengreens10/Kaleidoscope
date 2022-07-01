/* libc */
extern double sin(double d);
extern double system(string s);
extern double sqrt(double d);
extern double printf(string s1, string s2, double d1);
extern void gets(string s);
extern double strcmp(string s1, string s2);

def double main(double x) {
	set in = "                                                                                                                               ";
	
	while 1 {
		printf("%s", "$: ", 0);
		gets(in);
		system(in);
	};
	return 0;
}
